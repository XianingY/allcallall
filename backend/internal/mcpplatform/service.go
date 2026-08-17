package mcpplatform

import (
	"context"
	"regexp"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
)

const EventMCPExecutionTerminal = "mcp.execution.terminal"

const (
	sandboxMissingReceiptGrace    = 10 * time.Second
	mcpReconcileBaseDelay         = time.Second
	mcpReconcileMaximumDelay      = time.Minute
	mcpReconcilePersistenceWindow = 5 * time.Second
)

type MCPExecutionTerminalEvent struct {
	ExecutionID    string  `json:"execution_id"`
	MCPExecutionID uint64  `json:"mcp_execution_id"`
	AgentRunID     *uint64 `json:"agent_run_id,omitempty"`
	WorkflowRunID  *uint64 `json:"workflow_run_id,omitempty"`
	Status         string  `json:"status"`
}

type Service struct {
	db                *gorm.DB
	metrics           *metrics.CounterStore
	sandbox           SandboxClient
	outbox            *events.Store
	secrets           SecretStore
	capabilities      *CapabilityManager
	enabled           bool
	personalLimit     int
	organizationLimit int
	executionTimeout  time.Duration
	outputLimit       int
}

var mcpToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,160}$`)

func (s *Service) WithCapabilityManager(manager *CapabilityManager) *Service {
	s.capabilities = manager
	return s
}

func (s *Service) IssueForRun(ctx context.Context, organizationID, userID, conversationID uint64, runRef string) (string, error) {
	if s.capabilities == nil {
		return "", ErrInvalidCapability
	}
	return s.capabilities.IssueForRun(ctx, s, organizationID, userID, conversationID, runRef)
}

func NewService(db *gorm.DB, metricStore *metrics.CounterStore) *Service {
	return &Service{
		db:                db,
		metrics:           metricStore,
		secrets:           DisabledSecretStore{},
		enabled:           true,
		personalLimit:     DefaultPersonalInstallationLimit,
		organizationLimit: DefaultOrganizationLimit,
		executionTimeout:  DefaultExecutionTimeout,
		outputLimit:       DefaultOutputLimit,
	}
}

func (s *Service) WithEnabled(enabled bool) *Service {
	s.enabled = enabled
	return s
}

func (s *Service) WithSandbox(client SandboxClient) *Service {
	s.sandbox = client
	return s
}

func (s *Service) WithOutbox(store *events.Store) *Service {
	s.outbox = store
	return s
}

func (s *Service) WithSecretStore(store SecretStore) *Service {
	if store != nil {
		s.secrets = store
	}
	return s
}

func (s *Service) checkEnabled() error {
	if !s.enabled || s.db == nil {
		return ErrDisabled
	}
	return nil
}
