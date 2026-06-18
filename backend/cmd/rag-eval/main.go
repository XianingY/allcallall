package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

type ragEvalSource struct {
	Title          string  `json:"title"`
	Text           string  `json:"text"`
	ConversationID *uint64 `json:"conversation_id,omitempty"`
}

type ragEvalCase struct {
	Name                  string          `json:"name"`
	Query                 string          `json:"query"`
	UseVector             bool            `json:"use_vector"`
	Sources               []ragEvalSource `json:"sources"`
	ExpectedSourceTitles  []string        `json:"expected_source_titles"`
	ExpectedRetrievalMode string          `json:"expected_retrieval_mode"`
	RequireCitation       bool            `json:"require_citation"`
	RequiredSnippets      []string        `json:"required_snippets"`
}

type ragEvalHit struct {
	ChunkID       uint64  `json:"chunk_id"`
	SourceTitle   string  `json:"source_title"`
	RetrievalMode string  `json:"retrieval_mode"`
	Score         float64 `json:"score"`
	Snippet       string  `json:"snippet"`
}

type ragEvalResult struct {
	Name    string       `json:"name"`
	Passed  bool         `json:"passed"`
	Errors  []string     `json:"errors,omitempty"`
	Hits    []ragEvalHit `json:"hits"`
	Mode    string       `json:"mode"`
	Reason  string       `json:"fallback_reason,omitempty"`
	Elapsed string       `json:"elapsed"`
}

type ragEvalReport struct {
	Cases   int             `json:"cases"`
	Passed  int             `json:"passed"`
	Failed  int             `json:"failed"`
	Results []ragEvalResult `json:"results"`
}

func main() {
	fixturePath := flag.String("fixture", "./internal/agent/testdata/rag_eval_cases.json", "path to RAG eval cases JSON")
	flag.Parse()

	cases, err := loadRAGEvalCases(*fixturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load rag eval cases failed: %v\n", err)
		os.Exit(2)
	}
	report, err := runRAGEval(context.Background(), cases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run rag eval failed: %v\n", err)
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write rag eval report failed: %v\n", err)
		os.Exit(2)
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}

func loadRAGEvalCases(path string) ([]ragEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []ragEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func runRAGEval(ctx context.Context, cases []ragEvalCase) (ragEvalReport, error) {
	report := ragEvalReport{Cases: len(cases), Results: make([]ragEvalResult, 0, len(cases))}
	for i, item := range cases {
		started := time.Now()
		result, err := runRAGEvalCase(ctx, i+1, item)
		if err != nil {
			result = ragEvalResult{Name: item.Name, Errors: []string{err.Error()}}
		}
		result.Elapsed = time.Since(started).String()
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func runRAGEvalCase(ctx context.Context, index int, item ragEvalCase) (ragEvalResult, error) {
	db, err := openRAGEvalDB(index)
	if err != nil {
		return ragEvalResult{}, err
	}
	orgID := uint64(100 + index)
	userID := uint64(7)
	conversationID := uint64(1000 + index)
	if err := seedRAGEvalScope(db, orgID, userID, conversationID); err != nil {
		return ragEvalResult{}, err
	}
	outbox := events.NewStore(db)
	svc := knowledge.NewService(db).WithOutbox(outbox)
	vector := newRAGEvalVectorIndex()
	if item.UseVector {
		svc.WithEmbeddingProvider(vector).WithChunkIndexer(vector)
	}
	for _, source := range item.Sources {
		conversationPtr := source.ConversationID
		if conversationPtr != nil && *conversationPtr == 0 {
			conversationPtr = &conversationID
		}
		record, err := svc.CreateSource(ctx, orgID, userID, knowledge.CreateSourceInput{
			Kind:           models.RAGSourceKindManualText,
			Title:          source.Title,
			Text:           source.Text,
			ConversationID: conversationPtr,
		})
		if err != nil {
			return ragEvalResult{}, err
		}
		if err := svc.ProcessSourceIngest(ctx, record.ID); err != nil {
			return ragEvalResult{}, err
		}
		var chunks []models.RAGChunk
		if err := db.WithContext(ctx).Where("source_id = ?", record.ID).Order("id ASC").Find(&chunks).Error; err != nil {
			return ragEvalResult{}, err
		}
		for _, chunk := range chunks {
			if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
				return ragEvalResult{}, err
			}
		}
	}
	results, err := svc.Search(ctx, orgID, &conversationID, item.Query, 5)
	if err != nil {
		return ragEvalResult{}, err
	}
	eval := ragEvalResult{Name: item.Name, Hits: make([]ragEvalHit, 0, len(results))}
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
		eval.Hits = append(eval.Hits, ragEvalHit{
			ChunkID:       hit.Chunk.ID,
			SourceTitle:   sourceTitle,
			RetrievalMode: mode,
			Score:         float64(hit.Score),
			Snippet:       compactEvalSnippet(hit.Chunk.Content, 180),
		})
	}
	if len(results) == 0 {
		eval.Errors = append(eval.Errors, "no retrieval hits")
	}
	if item.ExpectedRetrievalMode != "" && eval.Mode != item.ExpectedRetrievalMode {
		eval.Errors = append(eval.Errors, fmt.Sprintf("retrieval mode got %q want %q", eval.Mode, item.ExpectedRetrievalMode))
	}
	for _, title := range item.ExpectedSourceTitles {
		if !seenTitles[title] {
			eval.Errors = append(eval.Errors, fmt.Sprintf("missing source hit %q", title))
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

func compactEvalSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-3]) + "..."
}

func evalHitSnippets(hits []ragEvalHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Snippet)
	}
	return out
}

type ragEvalVectorIndex struct {
	docs map[uint64]search.ContextChunkDocument
}

func newRAGEvalVectorIndex() *ragEvalVectorIndex {
	return &ragEvalVectorIndex{docs: map[uint64]search.ContextChunkDocument{}}
}

func (idx *ragEvalVectorIndex) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	return lexicalVector(text), nil
}

func (idx *ragEvalVectorIndex) IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error {
	_ = ctx
	idx.docs[doc.SourceID] = doc
	return nil
}

func (idx *ragEvalVectorIndex) SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	_ = ctx
	queryVector := query.QueryVector
	if len(queryVector) == 0 {
		return nil, nil
	}
	conversations := map[uint64]bool{}
	for _, id := range query.ConversationIDs {
		conversations[id] = true
	}
	sourceTypes := map[string]bool{}
	for _, value := range query.SourceTypes {
		sourceTypes[value] = true
	}
	results := make([]search.ContextChunkSearchResult, 0, len(idx.docs))
	for _, doc := range idx.docs {
		if doc.OrganizationID != query.OrganizationID {
			continue
		}
		if len(conversations) > 0 && !conversations[doc.ConversationID] {
			continue
		}
		if len(sourceTypes) > 0 && !sourceTypes[doc.SourceType] {
			continue
		}
		score := cosine(queryVector, doc.ContentVector)
		results = append(results, search.ContextChunkSearchResult{ContextChunkDocument: doc, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func lexicalVector(text string) []float32 {
	lowered := strings.ToLower(text)
	keywords := []string{"latency", "translation", "security", "budget", "pricing", "risk", "approval", "training"}
	vector := make([]float32, len(keywords))
	for i, keyword := range keywords {
		vector[i] = float32(strings.Count(lowered, keyword))
	}
	return vector
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
