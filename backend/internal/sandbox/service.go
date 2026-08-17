package sandbox

import (
	"context"
	"errors"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/rs/zerolog"
	"net"
	"os"
	"time"
)

// log 是该包内的包级最低限度日志器，用于记录关键清理/状态写入路径上被吞掉的错误。
// log is the package-level fallback logger for swallowed errors on critical cleanup/state-write paths.
var log = zerolog.New(os.Stderr).With().Timestamp().Logger()

var (
	ErrPrivateAddress      = errors.New("sandbox private network destination rejected")
	ErrImageRejected       = errors.New("sandbox image rejected")
	ErrExecutionConflict   = errors.New("sandbox execution id conflicts with a different request")
	ErrExecutionInProgress = errors.New("sandbox execution is already running")
	ErrInvalidExecution    = errors.New("invalid sandbox execution request")
	ErrReceiptUnavailable  = errors.New("sandbox execution receipt store unavailable")
)

const (
	defaultReceiptRetention     = 30 * 24 * time.Hour
	defaultReceiptStaleGrace    = 10 * time.Second
	terminalReceiptWriteTimeout = 5 * time.Second
)

type Runner interface {
	Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error)
	Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error)
}

type PreparedExecution interface {
	JobID() string
	Execute(context.Context) (mcpplatform.ExecutionResult, error)
	Close(context.Context) error
}

type PreparingRunner interface {
	PrepareExecution(context.Context, mcpplatform.ExecutionRequest) (PreparedExecution, error)
}

type ImageScanResult struct {
	Status string
	Report map[string]any
}

type ImageScanner interface {
	Scan(context.Context, string) (ImageScanResult, error)
}

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Service struct {
	runner            Runner
	ociRunner         Runner
	scanner           ImageScanner
	resolver          ipResolver
	receipts          *ReceiptStore
	receiptRetention  time.Duration
	receiptStaleGrace time.Duration
}

func NewService(runner Runner, scanner ImageScanner) *Service {
	return &Service{
		runner:            runner,
		scanner:           scanner,
		resolver:          net.DefaultResolver,
		receiptRetention:  defaultReceiptRetention,
		receiptStaleGrace: defaultReceiptStaleGrace,
	}
}

func (s *Service) WithReceiptStore(store *ReceiptStore) *Service {
	s.receipts = store
	return s
}

// WithOCIRunner installs the only execution path allowed to launch digest-pinned
// user images. OCI requests never fall back to the shared HTTPS Runner.
func (s *Service) WithOCIRunner(runner Runner) *Service {
	s.ociRunner = runner
	return s
}
