package knowledge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

type fakeEmbedder struct {
	err error
}

func (f fakeEmbedder) CreateEmbedding(context.Context, string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

type fakeChunkIndexer struct {
	docs map[string]search.ContextChunkDocument
	err  error
}

func (f *fakeChunkIndexer) IndexChunk(_ context.Context, doc search.ContextChunkDocument) error {
	if f.err != nil {
		return f.err
	}
	if f.docs == nil {
		f.docs = map[string]search.ContextChunkDocument{}
	}
	f.docs[doc.ID] = doc
	return nil
}

func (f *fakeChunkIndexer) SearchChunks(_ context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	var out []search.ContextChunkSearchResult
	for _, doc := range f.docs {
		if doc.OrganizationID != query.OrganizationID || doc.SourceType != "knowledge" {
			continue
		}
		out = append(out, search.ContextChunkSearchResult{ContextChunkDocument: doc, Score: 1.25})
	}
	return out, nil
}

type fakeHybridChunkIndexer struct {
	fakeChunkIndexer
}

func (f *fakeHybridChunkIndexer) SearchChunksHybrid(_ context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	var out []search.ContextChunkSearchResult
	for _, doc := range f.docs {
		if doc.OrganizationID != query.OrganizationID || doc.SourceType != "knowledge" {
			continue
		}
		out = append(out, search.ContextChunkSearchResult{
			ContextChunkDocument: doc,
			Score:                0.032,
			RetrievalMode:        models.RAGRetrievalModeHybridRRF,
			BM25Rank:             1,
			VectorRank:           2,
			RRFScore:             0.032,
			BM25Score:            8.5,
			VectorScore:          1.3,
		})
	}
	return out, nil
}

// SearchChunks mirrors the production ElasticsearchIndexer: a hybrid-capable
// indexer routes SearchChunks through the dual-channel (RRF) path by default.
func (f *fakeHybridChunkIndexer) SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	return f.SearchChunksHybrid(ctx, query)
}

type reverseReranker struct{}

func (reverseReranker) Rerank(_ context.Context, input search.RerankInput) ([]search.RerankResult, error) {
	out := make([]search.RerankResult, 0, len(input.Candidates))
	for i := len(input.Candidates) - 1; i >= 0; i-- {
		out = append(out, search.RerankResult{
			ID:           input.Candidates[i].ID,
			RerankScore:  float64(len(input.Candidates) - i),
			RerankReason: "test_reverse",
			FinalRank:    len(out) + 1,
		})
	}
	return out, nil
}

func TestChunkTextUsesOverlapAndDedupesWithinVersion(t *testing.T) {
	input := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu"
	chunks := ChunkText(input, 24, 6)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[1].StartOffset >= chunks[0].EndOffset {
		t.Fatalf("expected overlap, first=%+v second=%+v", chunks[0], chunks[1])
	}

	dupes := ChunkText("same same same", 100, 10)
	if len(dupes) != 1 {
		t.Fatalf("expected one deduped chunk, got %d", len(dupes))
	}
	if dupes[0].ContentHash == "" || dupes[0].Keywords == "" {
		t.Fatalf("expected hash and keywords: %+v", dupes[0])
	}
}

func TestHybridSearchReturnsRRFMetadata(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	indexer := &fakeHybridChunkIndexer{}
	svc := NewService(db).
		WithOutbox(events.NewStore(db)).
		WithEmbeddingProvider(fakeEmbedder{}).
		WithChunkIndexer(indexer)

	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Hybrid playbook",
		ConversationID: &conversationID,
		Text:           "Hybrid retrieval should combine vector relevance with exact keyword evidence.",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatalf("process ingest: %v", err)
	}
	var chunks []models.RAGChunk
	if err := db.Where("source_id = ?", source.ID).Find(&chunks).Error; err != nil {
		t.Fatal(err)
	}
	for _, chunk := range chunks {
		if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
			t.Fatalf("index chunk %d: %v", chunk.ID, err)
		}
	}
	results, err := svc.Search(ctx, orgID, &conversationID, "hybrid keyword evidence", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected hybrid results")
	}
	got := results[0]
	if got.RetrievalMode != models.RAGRetrievalModeHybridRRF || got.BM25Rank != 1 || got.VectorRank != 2 || got.RRFScore == 0 {
		t.Fatalf("missing hybrid metadata: %+v", got)
	}
}

func TestChunkIndexSupportsBM25WithoutEmbeddingProvider(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	indexer := &fakeChunkIndexer{}
	svc := NewService(db).WithOutbox(events.NewStore(db)).WithChunkIndexer(indexer)

	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Agent tool approval policy",
		ConversationID: &conversationID,
		Text:           "Write and unknown-risk tools require administrator approval.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	var chunk models.RAGChunk
	if err := db.Where("source_id = ?", source.ID).Take(&chunk).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
		t.Fatal(err)
	}
	doc := indexer.docs[KnowledgeDocumentID(chunk.ID)]
	if doc.SourceType != "knowledge" || doc.Content == "" || len(doc.ContentVector) != 0 {
		t.Fatalf("BM25-only document was not indexed correctly: %+v", doc)
	}
	if err := db.Take(&chunk, chunk.ID).Error; err != nil {
		t.Fatal(err)
	}
	if chunk.IndexStatus != models.RAGChunkIndexStatusIndexed {
		t.Fatalf("expected indexed chunk, got %s", chunk.IndexStatus)
	}
}

func TestUnchangedReingestRequeuesSkippedChunks(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	svc := NewService(db).WithOutbox(events.NewStore(db))
	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Recoverable index source",
		ConversationID: &conversationID,
		Text:           "Retry unchanged knowledge after an indexer becomes available.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	var chunk models.RAGChunk
	if err := db.Where("source_id = ?", source.ID).Take(&chunk).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Take(&chunk, chunk.ID).Error; err != nil {
		t.Fatal(err)
	}
	if chunk.IndexStatus != models.RAGChunkIndexStatusSkipped {
		t.Fatalf("expected skipped chunk before reingest, got %s", chunk.IndexStatus)
	}
	var before int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ?", EventChunkIndexRequested).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Take(&chunk, chunk.ID).Error; err != nil {
		t.Fatal(err)
	}
	if chunk.IndexStatus != models.RAGChunkIndexStatusPending {
		t.Fatalf("unchanged reingest did not reset chunk status: %s", chunk.IndexStatus)
	}
	var after int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ?", EventChunkIndexRequested).Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("unchanged reingest should enqueue one chunk retry: before=%d after=%d", before, after)
	}
}

func TestDuplicateConfirmationFiltersCanonicalRetrieval(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	svc := NewService(db).WithOutbox(events.NewStore(db))

	first, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Canonical playbook",
		ConversationID: &conversationID,
		Text:           "The escalation owner must review renewal risk before the pilot deadline.",
	})
	if err != nil {
		t.Fatalf("create first source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, first.ID); err != nil {
		t.Fatalf("ingest first: %v", err)
	}
	second, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Duplicate playbook",
		ConversationID: &conversationID,
		Text:           "The escalation owner must review renewal risk before the pilot deadline.",
	})
	if err != nil {
		t.Fatalf("create second source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, second.ID); err != nil {
		t.Fatalf("ingest second: %v", err)
	}
	duplicates, err := svc.ListDuplicateCandidates(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("list duplicate candidates: %v", err)
	}
	if len(duplicates) == 0 || duplicates[0].DuplicateKind != models.RAGSourceDuplicateKindExact {
		t.Fatalf("expected exact duplicate candidate, got %+v", duplicates)
	}
	if err := svc.DecideDuplicateCandidate(ctx, orgID, userID, duplicates[0].ID, "confirm"); err != nil {
		t.Fatalf("confirm duplicate: %v", err)
	}
	results, err := svc.Search(ctx, orgID, &conversationID, "renewal risk pilot", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected canonical fallback result")
	}
	for _, result := range results {
		if result.Source.ID == second.ID {
			t.Fatalf("confirmed duplicate should be filtered: %+v", result)
		}
	}
}

func TestManualSourceIngestIndexesAndSearches(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	indexer := &fakeChunkIndexer{}
	svc := NewService(db).
		WithOutbox(events.NewStore(db)).
		WithEmbeddingProvider(fakeEmbedder{}).
		WithChunkIndexer(indexer)

	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Support playbook",
		ConversationID: &conversationID,
		Text:           "Translation latency target is under 500ms. Escalate if the security review blocks the pilot.",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatalf("process ingest: %v", err)
	}

	var ready models.RAGSource
	if err := db.Take(&ready, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ready.Status != models.RAGSourceStatusReady || ready.ActiveVersionID == nil {
		t.Fatalf("source not ready: %+v", ready)
	}

	var chunks []models.RAGChunk
	if err := db.Where("source_id = ?", source.ID).Order("chunk_index ASC").Find(&chunks).Error; err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatalf("expected chunks")
	}
	for _, chunk := range chunks {
		if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
			t.Fatalf("index chunk %d: %v", chunk.ID, err)
		}
	}
	if len(indexer.docs) != len(chunks) {
		t.Fatalf("expected %d indexed docs, got %d", len(chunks), len(indexer.docs))
	}

	results, err := svc.Search(ctx, orgID, &conversationID, "security pilot latency", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].RetrievalMode != models.RAGRetrievalModeVector {
		t.Fatalf("expected vector results, got %+v", results)
	}
	if results[0].Source.Title != "Support playbook" || results[0].Version.Version != 1 {
		t.Fatalf("unexpected source metadata: %+v", results[0])
	}
}

func TestSearchFallsBackToSQLWithoutIndexer(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	svc := NewService(db).WithOutbox(events.NewStore(db))
	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Fallback source",
		ConversationID: &conversationID,
		Text:           "Budget approval depends on legal review and security documentation.",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatalf("process ingest: %v", err)
	}
	results, err := svc.Search(ctx, orgID, &conversationID, "legal security", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected fallback results")
	}
	if results[0].RetrievalMode != models.RAGRetrievalModeSQLFallback || results[0].FallbackReason != "indexer_unavailable" {
		t.Fatalf("unexpected fallback metadata: %+v", results[0])
	}
}

func TestSearchAppliesRerankerMetadata(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	svc := NewService(db).WithOutbox(events.NewStore(db)).WithReranker(reverseReranker{})
	for _, spec := range []struct {
		title string
		text  string
	}{
		{title: "First source", text: "Security approval notes for baseline ordering."},
		{title: "Second source", text: "Security approval risk and follow-up notes."},
	} {
		source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
			Kind:           models.RAGSourceKindManualText,
			Title:          spec.title,
			ConversationID: &conversationID,
			Text:           spec.text,
		})
		if err != nil {
			t.Fatalf("create source: %v", err)
		}
		if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
			t.Fatalf("process ingest: %v", err)
		}
	}
	results, err := svc.Search(ctx, orgID, &conversationID, "security approval", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].FinalRank != 1 || results[0].RerankScore == 0 || results[0].RerankReason != "test_reverse" {
		t.Fatalf("missing rerank metadata: %+v", results[0])
	}
}

func TestChunkIndexFailureIsRetryable(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, conversationID := seedKnowledgeAccess(t, db)
	svc := NewService(db).
		WithOutbox(events.NewStore(db)).
		WithEmbeddingProvider(fakeEmbedder{err: errors.New("embedding unavailable")}).
		WithChunkIndexer(&fakeChunkIndexer{})
	source, err := svc.CreateSource(ctx, orgID, userID, CreateSourceInput{
		Kind:           models.RAGSourceKindManualText,
		Title:          "Retry source",
		ConversationID: &conversationID,
		Text:           "Retry this chunk when embedding fails.",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := svc.ProcessSourceIngest(ctx, source.ID); err != nil {
		t.Fatalf("process ingest: %v", err)
	}
	var chunk models.RAGChunk
	if err := db.Where("source_id = ?", source.ID).Take(&chunk).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessChunkIndex(ctx, chunk.ID); err == nil {
		t.Fatalf("expected indexing error")
	}
	if err := db.Take(&chunk, chunk.ID).Error; err != nil {
		t.Fatal(err)
	}
	if chunk.IndexStatus != models.RAGChunkIndexStatusFailed || chunk.LastError == "" {
		t.Fatalf("expected failed chunk status, got %+v", chunk)
	}
}

func TestRetryDeadLetterRequeuesRAGEvent(t *testing.T) {
	ctx := context.Background()
	db := newKnowledgeTestDB(t)
	orgID, userID, _ := seedKnowledgeAccess(t, db)
	svc := NewService(db)
	row := models.EventOutbox{
		AggregateType:  "rag_chunk",
		AggregateID:    99,
		Event:          EventChunkIndexRequested,
		PayloadJSON:    `{"organization_id":1,"chunk_id":99}`,
		IdempotencyKey: "dead-letter-test",
		Status:         models.EventOutboxStatusFailed,
		Attempts:       3,
		LastError:      "boom",
		AvailableAt:    timePtr(time.Now().UTC().Add(time.Hour)),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RetryDeadLetter(ctx, orgID, userID, row.ID); err != nil {
		t.Fatalf("retry dead letter: %v", err)
	}
	var updated models.EventOutbox
	if err := db.Take(&updated, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.EventOutboxStatusPending || updated.Attempts != 0 || updated.LastError != "" || updated.AvailableAt != nil {
		t.Fatalf("dead letter was not requeued: %+v", updated)
	}
}

func newKnowledgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "knowledge.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	return db
}

func seedKnowledgeAccess(t *testing.T, db *gorm.DB) (uint64, uint64, uint64) {
	t.Helper()
	org := models.Organization{Name: "Test Org", Slug: "test-org", CreatedBy: 7}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	member := models.OrganizationMember{OrganizationID: org.ID, UserID: 7, Role: models.OrganizationRoleOwner, JoinedAt: time.Now().UTC()}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	conv := models.Conversation{OrganizationID: org.ID, Type: models.ConversationTypeChannel, Title: "Knowledge thread", Status: models.ConversationStatusOpen, CreatedBy: 7}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	convMember := models.ConversationMember{ConversationID: conv.ID, UserID: 7, Role: models.OrganizationRoleOwner}
	if err := db.Create(&convMember).Error; err != nil {
		t.Fatal(err)
	}
	return org.ID, uint64(7), conv.ID
}

func timePtr(value time.Time) *time.Time {
	return &value
}
