package agent

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

var (
	ErrConversationAccessDenied = errors.New("conversation access denied")
	ErrAgentRunNotFound         = errors.New("agent run not found")
)

func isDeferredRunExecution(err error) bool {
	return errors.Is(err, ErrCheckpointExecutionBusy) || errors.Is(err, mcpplatform.ErrExecutionInProgress)
}

const (
	agentRunMaxAttempts   = 3
	agentRunLeaseDuration = 5 * time.Minute
)

type ChunkIndexer interface {
	IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error
	SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type StreamPublisher interface {
	PublishToken(ctx context.Context, runID uint64, token string) error
}

type KnowledgeRetriever interface {
	Search(ctx context.Context, organizationID uint64, conversationID *uint64, query string, limit int) ([]knowledge.SearchResult, error)
}

type ToolCapabilityProvider interface {
	IssueForRun(ctx context.Context, organizationID, userID, conversationID uint64, runRef string) (string, error)
}

type Service struct {
	db                 *gorm.DB
	metrics            metrics.Recorder
	planner            Planner
	outbox             *events.Store
	indexer            ChunkIndexer
	knowledgeRetriever KnowledgeRetriever
	reranker           search.Reranker
	streamPublisher    StreamPublisher
	strictProvider     bool
	workflowRuntime    WorkflowRuntime
	toolCapabilities   ToolCapabilityProvider
	mcpPlatform        *mcpplatform.Service
}

type RunInput struct {
	ConversationID uint64
	Goal           string
	Role           string
	IdempotencyKey string
}

type RunResult struct {
	Run         models.AgentRun        `json:"run"`
	Steps       []models.AgentStep     `json:"steps"`
	ToolCalls   []models.AgentToolCall `json:"tool_calls"`
	Trace       []TraceEvent           `json:"trace"`
	Citations   []Citation             `json:"citations"`
	ActionItems []string               `json:"action_items"`
	RiskFlags   []string               `json:"risk_flags"`
}

type conversationContext struct {
	Conversation              models.Conversation
	Notes                     []models.ConversationNote
	Messages                  []models.Message
	Rooms                     []models.CallRoom
	Members                   []models.ConversationMember
	Memories                  []models.AgentMemory
	Followups                 []models.CallFollowup
	TranscriptSegments        []models.CallTranscriptSegment
	MeetingTranscriptSegments []models.MeetingTranscriptSegment
	ContactProfile            *models.ContactProfile
	ContextChunks             []RetrievedContextChunk
	MeetingContext            meetingContextSummary
}

type meetingContextSummary struct {
	LatestCallID                  string
	TranscriptSegmentCount        int
	LatestTranscriptAt            *time.Time
	LatestFollowupPresent         bool
	MeetingTranscriptionStatus    string
	MeetingTranscriptSegmentCount int
	LatestMeetingTranscriptAt     *time.Time
}

type conversationMemoryInput struct {
	Key         string
	Summary     string
	ActionItems []string
	NextStep    string
	RiskFlags   []string
	MemoryType  string
	Importance  int
	SourceType  string
	SourceRefID uint64
}

func NewService(db *gorm.DB, recorders ...metrics.Recorder) *Service {
	var metrics metrics.Recorder
	if len(recorders) > 0 {
		metrics = recorders[0]
	}
	reranker, _ := search.NewRerankerFromEnv()
	return &Service{
		db:              db,
		metrics:         metrics,
		planner:         RulesPlanner{},
		outbox:          events.NewStore(db),
		reranker:        reranker,
		strictProvider:  AgentProviderStrictFromEnv(),
		workflowRuntime: NewWorkflowRuntimeFromEnv(),
	}
}

func (s *Service) WithStrictProvider(strict bool) *Service {
	s.strictProvider = strict
	return s
}

func (s *Service) WithPlanner(p Planner) *Service {
	s.planner = p
	return s
}

func (s *Service) WithChunkIndexer(i ChunkIndexer) *Service {
	s.indexer = i
	return s
}

func (s *Service) WithKnowledgeRetriever(r KnowledgeRetriever) *Service {
	s.knowledgeRetriever = r
	return s
}

func (s *Service) WithReranker(r search.Reranker) *Service {
	s.reranker = r
	return s
}

func (s *Service) WithOutbox(outbox *events.Store) {
	if outbox != nil {
		s.outbox = outbox
	}
}

func (s *Service) WithStreamPublisher(p StreamPublisher) *Service {
	s.streamPublisher = p
	return s
}

func (s *Service) WithWorkflowRuntime(runtime WorkflowRuntime) *Service {
	s.workflowRuntime = runtime
	return s
}

func (s *Service) WithToolCapabilityProvider(provider ToolCapabilityProvider) *Service {
	s.toolCapabilities = provider
	return s
}

func (s *Service) WithMCPPlatform(platform *mcpplatform.Service) *Service {
	s.mcpPlatform = platform
	return s
}

// DB returns the underlying gorm.DB.
func (s *Service) DB() *gorm.DB {
	return s.db
}
