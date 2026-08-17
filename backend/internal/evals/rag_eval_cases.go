package evals

import (
	"context"
	"fmt"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runRAGEvalCase(ctx context.Context, index int, item RAGEvalCase, opts RAGEvalOptions) (RAGEvalResult, error) {
	db, err := openRAGEvalDB(index)
	if err != nil {
		return RAGEvalResult{}, err
	}
	orgID := uint64(100 + index)
	userID := uint64(7)
	conversationID := uint64(1000 + index)
	if err := seedRAGEvalScope(db, orgID, userID, conversationID); err != nil {
		return RAGEvalResult{}, err
	}
	outbox := events.NewStore(db)
	svc := knowledge.NewService(db).WithOutbox(outbox).WithReranker(nil)
	vector := newRAGEvalVectorIndex()
	if item.UseVector {
		svc.WithEmbeddingProvider(vector).WithChunkIndexer(vector)
	}
	for i, source := range item.Sources {
		chunks, err := seedRAGEvalSource(ctx, db, orgID, userID, conversationID, i+1, source)
		if err != nil {
			return RAGEvalResult{}, err
		}
		for _, chunk := range chunks {
			if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
				return RAGEvalResult{}, err
			}
		}
	}
	baselineResults, err := svc.Search(ctx, orgID, &conversationID, item.Query, 5)
	if err != nil {
		return RAGEvalResult{}, err
	}
	results := baselineResults
	if opts.EnableRerank {
		reranker := opts.Reranker
		if reranker == nil {
			reranker = search.NewRulesReranker()
		}
		results, err = svc.WithReranker(reranker).Search(ctx, orgID, &conversationID, item.Query, 5)
		if err != nil {
			return RAGEvalResult{}, err
		}
	}
	eval := RAGEvalResult{Name: item.Name, Hits: ragHitsFromResults(results)}
	if opts.EnableRerank {
		eval.BaselineHits = ragHitsFromResults(baselineResults)
	}
	seenTitles := map[string]bool{}
	for _, hit := range results {
		sourceTitle := ""
		if hit.Source.ID != 0 {
			sourceTitle = hit.Source.Title
		}
		seenTitles[sourceTitle] = true
		mode := hit.RetrievalMode
		if eval.Mode == "" {
			eval.Mode = mode
			eval.Reason = hit.FallbackReason
		}
	}
	if len(results) == 0 && !item.ExpectedNoAnswer {
		eval.Errors = append(eval.Errors, "no retrieval hits")
	}
	if item.ExpectedRetrievalMode != "" && eval.Mode != item.ExpectedRetrievalMode {
		eval.Errors = append(eval.Errors, fmt.Sprintf("retrieval mode got %q want %q", eval.Mode, item.ExpectedRetrievalMode))
	}
	if !item.ExpectedNoAnswer {
		for _, title := range item.ExpectedSourceTitles {
			if !seenTitles[title] {
				eval.Errors = append(eval.Errors, fmt.Sprintf("missing source hit %q", title))
			}
		}
	}
	if item.RequireCitation {
		for _, hit := range results {
			if hit.Chunk.ID == 0 || hit.Source.ID == 0 || strings.TrimSpace(hit.Chunk.Content) == "" {
				eval.Errors = append(eval.Errors, "retrieval hit missing citation fields")
				break
			}
		}
	}
	combined := strings.ToLower(strings.Join(evalHitSnippets(eval.Hits), " "))
	for _, snippet := range item.RequiredSnippets {
		if !strings.Contains(combined, strings.ToLower(snippet)) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("grounding snippet missing %q", snippet))
		}
	}
	return eval, nil
}

func openRAGEvalDB(index int) (*gorm.DB, error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("allcallall-rag-eval-%d-", index))
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "rag-eval.db")+"?_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.RAGSourceGroup{},
		&models.RAGSourceDuplicate{},
		&models.RAGSource{},
		&models.RAGSourceVersion{},
		&models.RAGChunk{},
		&models.EventOutbox{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func seedRAGEvalScope(db *gorm.DB, orgID, userID, conversationID uint64) error {
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.Organization{ID: orgID, Name: "RAG Eval Org", CreatedBy: userID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrganizationRoleOwner, JoinedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.Conversation{ID: conversationID, OrganizationID: orgID, Type: models.ConversationTypeChannel, Title: "RAG Eval", Status: models.ConversationStatusOpen, CreatedBy: userID}).Error; err != nil {
			return err
		}
		return tx.Create(&models.ConversationMember{ConversationID: conversationID, UserID: userID, Role: models.OrganizationRoleOwner}).Error
	})
}

func seedRAGEvalSource(ctx context.Context, db *gorm.DB, orgID, userID, conversationID uint64, index int, item RAGEvalSource) ([]models.RAGChunk, error) {
	now := time.Now().UTC()
	conversationPtr := item.ConversationID
	if conversationPtr != nil && *conversationPtr == 0 {
		conversationPtr = &conversationID
	}
	var chunks []models.RAGChunk
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		source := models.RAGSource{
			OrganizationID: orgID,
			ConversationID: conversationPtr,
			CreatedBy:      userID,
			Kind:           models.RAGSourceKindManualText,
			Title:          item.Title,
			AuthorityScore: 0.6,
			AuthorityLabel: "eval",
			DedupeStatus:   models.RAGSourceDedupeStatusUnique,
			Status:         models.RAGSourceStatusReady,
		}
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		version := models.RAGSourceVersion{
			OrganizationID: orgID,
			SourceID:       source.ID,
			Version:        1,
			ContentHash:    knowledge.HashText(item.Text),
			NormalizedHash: knowledge.HashText(knowledge.NormalizeText(item.Text)),
			SimHash64:      int64(index),
			RawText:        item.Text,
			Status:         models.RAGSourceVersionStatusActive,
			ChunkCount:     1,
			ActivatedAt:    &now,
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Update("active_version_id", version.ID).Error; err != nil {
			return err
		}
		chunk := models.RAGChunk{
			OrganizationID:  orgID,
			ConversationID:  conversationPtr,
			SourceID:        source.ID,
			SourceVersionID: version.ID,
			ChunkIndex:      0,
			StartOffset:     0,
			EndOffset:       len([]rune(item.Text)),
			ContentHash:     knowledge.HashText(fmt.Sprintf("%d:%s", index, item.Text)),
			Content:         item.Text,
			Keywords:        strings.Join(strings.Fields(knowledge.NormalizeText(item.Text)), " "),
			IndexStatus:     models.RAGChunkIndexStatusPending,
		}
		if err := tx.Create(&chunk).Error; err != nil {
			return err
		}
		chunks = append(chunks, chunk)
		return nil
	})
	return chunks, err
}

func compactEvalSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-3]) + "..."
}

func evalHitSnippets(hits []RAGEvalHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Snippet)
	}
	return out
}
