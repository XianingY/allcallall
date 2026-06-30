package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"gorm.io/gorm"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	pdf "github.com/ledongthuc/pdf"
)


const (
	EventSourceIngestRequested = "rag.source.ingest_requested"
	EventChunkIndexRequested   = "rag.chunk.index_requested"

	MaxUploadBytes          int64 = 5 * 1024 * 1024
	MaxURLBytes             int64 = 2 * 1024 * 1024
	defaultChunkSize              = 900
	defaultChunkOverlap           = 120
	defaultSearchLimit            = 8
	nearDuplicateHammingMax       = 6
)

var (
	ErrAccessDenied        = errors.New("knowledge access denied")
	ErrSourceNotFound      = errors.New("knowledge source not found")
	ErrUnsupportedSource   = errors.New("unsupported knowledge source")
	ErrUnsupportedFileType = errors.New("unsupported knowledge file type")
)

type ChunkIndexer interface {
	IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error
	SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type BM25ChunkSearcher interface {
	SearchChunksBM25(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type HybridChunkSearcher interface {
	SearchChunksHybrid(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type EmbeddingProvider interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type Service struct {
	repo    *Repository
	outbox  *events.Store
	indexer ChunkIndexer
	embedder EmbeddingProvider
	reranker search.Reranker
	client   *http.Client
}

type CreateSourceInput struct {
	Kind           string
	Title          string
	ConversationID *uint64
	Text           string
	URL            string
	FileName       string
	ContentType    string
	FileBytes      []byte
}

type ListSourcesFilter struct {
	ConversationID *uint64
	Status         string
}

type SearchResult struct {
	Chunk          models.RAGChunk
	Source         models.RAGSource
	Version        models.RAGSourceVersion
	Score          int
	RetrievalMode  string
	FallbackReason string
	BM25Rank       int
	VectorRank     int
	RRFScore       float64
	BM25Score      float64
	VectorScore    float64
	RerankScore    float64
	RerankReason   string
	FinalRank      int
}

type ChunkSpec struct {
	Index       int
	StartOffset int
	EndOffset   int
	Content     string
	ContentHash string
	Keywords    string
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithRepository(NewRepository(db))
}

func NewServiceWithRepository(repo *Repository) *Service {
	reranker, _ := search.NewRerankerFromEnv()
	return &Service{
		repo:     repo,
		outbox:   events.NewStore(repo.DB()),
		reranker: reranker,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) WithOutbox(outbox *events.Store) *Service {
	if outbox != nil {
		s.outbox = outbox
	}
	return s
}

func (s *Service) WithChunkIndexer(indexer ChunkIndexer) *Service {
	s.indexer = indexer
	return s
}

func (s *Service) WithEmbeddingProvider(provider EmbeddingProvider) *Service {
	s.embedder = provider
	return s
}

func (s *Service) WithReranker(reranker search.Reranker) *Service {
	s.reranker = reranker
	return s
}

func (s *Service) ListRAGDeadLetters(ctx context.Context, organizationID, userID uint64) ([]models.EventOutbox, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListRAGDeadLetters(ctx)
}

func (s *Service) RetryDeadLetter(ctx context.Context, organizationID, userID, eventID uint64) error {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return err
	}
	row, err := s.repo.GetDeadLetterByID(ctx, eventID)
	if err != nil {
		return err
	}
	if !outboxPayloadMatchesOrg(row.PayloadJSON, organizationID) {
		return ErrAccessDenied
	}
	return s.repo.ResetDeadLetterToPending(ctx, row.ID, time.Now().UTC())
}

func (s *Service) loadSourceVersion(ctx context.Context, sourceID, versionID uint64) (models.RAGSource, models.RAGSourceVersion, error) {
	var source models.RAGSource
	if err := s.repo.DB().WithContext(ctx).Where("id = ?", sourceID).Take(&source).Error; err != nil {
		return models.RAGSource{}, models.RAGSourceVersion{}, err
	}
	var version models.RAGSourceVersion
	if err := s.repo.DB().WithContext(ctx).Where("id = ?", versionID).Take(&version).Error; err != nil {
		return models.RAGSource{}, models.RAGSourceVersion{}, err
	}
	return source, version, nil
}

func (s *Service) ensureOrganizationMember(ctx context.Context, organizationID, userID uint64) error {
	count, err := s.repo.CountOrganizationMembers(ctx, organizationID, userID)
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) ensureOrganizationAdmin(ctx context.Context, organizationID, userID uint64) error {
	member, err := s.repo.GetOrganizationMember(ctx, organizationID, userID)
	if err != nil {
		return ErrAccessDenied
	}
	if member.Role != models.OrganizationRoleOwner && member.Role != models.OrganizationRoleAdmin {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	count, err := s.repo.CountConversationMembers(ctx, organizationID, userID, conversationID)
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrAccessDenied
	}
	return nil
}

func secureRedirectPolicy(next func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if _, err := validateFetchURL(req.URL.String()); err != nil {
			return fmt.Errorf("unsafe redirect target: %w", err)
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
}

func extractPDFText(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	doc, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	plain, err := doc.GetPlainText()
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(plain)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ExtractHTMLText(raw string) string {
	value := raw
	value = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, " ")
	return NormalizeText(html.UnescapeString(value))
}

func HashText(input string) string {
	sum := sha256.Sum256([]byte(NormalizeText(input)))
	return hex.EncodeToString(sum[:])
}

func KnowledgeDocumentID(chunkID uint64) string {
	return fmt.Sprintf("knowledge:%d", chunkID)
}

func ChunkText(input string, chunkSize, overlap int) []ChunkSpec {
	input = NormalizeText(input)
	if input == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = defaultChunkOverlap
	}
	runes := []rune(input)
	var out []ChunkSpec
	seen := map[string]bool{}
	for start, index := 0, 0; start < len(runes); index++ {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		content := NormalizeText(string(runes[start:end]))
		hash := HashText(content)
		if content != "" && !seen[hash] {
			seen[hash] = true
			out = append(out, ChunkSpec{
				Index:       len(out),
				StartOffset: start,
				EndOffset:   end,
				Content:     content,
				ContentHash: hash,
				Keywords:    strings.Join(extractKeywords(content), " "),
			})
		}
		if end == len(runes) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = end
		}
	}
	return out
}

func rankSQLFallback(chunks map[uint64]models.RAGChunk, sources map[uint64]models.RAGSource, versions map[uint64]models.RAGSourceVersion, query string, limit int, reason string) []SearchResult {
	tokens := extractKeywords(query)
	out := make([]SearchResult, 0, len(chunks))
	seen := map[string]bool{}
	for _, chunk := range chunks {
		if seen[chunk.ContentHash] {
			continue
		}
		score := scoreChunk(tokens, chunk)
		if score == 0 && len(tokens) > 0 {
			continue
		}
		if score == 0 {
			score = 1
		}
		seen[chunk.ContentHash] = true
		out = append(out, SearchResult{
			Chunk:          chunk,
			Source:         sources[chunk.SourceID],
			Version:        versions[chunk.SourceVersionID],
			Score:          score,
			RetrievalMode:  models.RAGRetrievalModeSQLFallback,
			FallbackReason: reason,
		})
	}
	if len(out) == 0 && len(chunks) > 0 {
		for _, chunk := range chunks {
			if seen[chunk.ContentHash] {
				continue
			}
			seen[chunk.ContentHash] = true
			out = append(out, SearchResult{
				Chunk:          chunk,
				Source:         sources[chunk.SourceID],
				Version:        versions[chunk.SourceVersionID],
				Score:          1,
				RetrievalMode:  models.RAGRetrievalModeSQLFallback,
				FallbackReason: reason,
			})
			if len(out) >= limit {
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Chunk.UpdatedAt.After(out[j].Chunk.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func scoreChunk(tokens []string, chunk models.RAGChunk) int {
	if len(tokens) == 0 {
		return 0
	}
	keywords := map[string]bool{}
	for _, keyword := range strings.Fields(strings.ToLower(chunk.Keywords)) {
		keywords[keyword] = true
	}
	content := strings.ToLower(chunk.Content)
	score := 0
	for _, token := range tokens {
		if keywords[token] {
			score += 5
		}
		if strings.Contains(content, token) {
			score += 2
		}
	}
	return score
}

func extractKeywords(input string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
		"for": true, "in": true, "is": true, "of": true, "on": true, "or": true, "the": true,
		"to": true, "with": true, "current": true, "summarize": true, "summary": true,
	}
	seen := map[string]bool{}
	var out []string
	addToken := func(token string) {
		token = strings.ToLower(strings.TrimSpace(token))
		if len([]rune(token)) < 2 || stopWords[token] || seen[token] {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		addToken(word.String())
		word.Reset()
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		for size := 2; size <= 4; size++ {
			if len(cjk) < size {
				continue
			}
			for i := 0; i+size <= len(cjk); i++ {
				addToken(string(cjk[i : i+size]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range input {
		if isCJKRune(r) {
			flushWord()
			cjk = append(cjk, r)
			continue
		}
		flushCJK()
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(unicode.ToLower(r))
			continue
		}
		flushWord()
	}
	flushWord()
	flushCJK()
	return out
}

func isCJKRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xF900 && r <= 0xFAFF)
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	return false
}

func normalizeContentType(value string) string {
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mediaType)
}

func outboxPayloadMatchesOrg(raw string, organizationID uint64) bool {
	var payload struct {
		OrganizationID uint64 `json:"organization_id"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	return payload.OrganizationID == organizationID
}
