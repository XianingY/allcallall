package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type fakeRunner struct {
	mu          sync.Mutex
	validations int
	executions  int
	execute     func(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error)
}

func (f *fakeRunner) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validations++
	return mcpplatform.ValidationResult{Tools: []mcpplatform.DiscoveredTool{{Name: "search", Risk: "read"}}}, nil
}

func (f *fakeRunner) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	f.mu.Lock()
	f.executions++
	execute := f.execute
	f.mu.Unlock()
	if execute != nil {
		return execute(ctx, request)
	}
	return mcpplatform.ExecutionResult{ExecutionID: request.ExecutionID, JobID: request.ExecutionID, Output: map[string]any{"ok": true}}, nil
}

func (f *fakeRunner) executionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.executions
}

type fixedScanner struct {
	status string
}

type fixedResolver struct {
	addresses []net.IPAddr
}

func (r fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, nil
}

func (s fixedScanner) Scan(context.Context, string) (ImageScanResult, error) {
	return ImageScanResult{Status: s.status, Report: map[string]any{"critical_vulnerabilities": 1}}, nil
}

func newTestService(runner Runner, scanner ImageScanner) *Service {
	return NewService(runner, scanner).WithOCIRunner(runner)
}

type receiptAwarePreparingRunner struct {
	store    *ReceiptStore
	executed atomic.Bool
}

func (r *receiptAwarePreparingRunner) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	return mcpplatform.ValidationResult{}, nil
}

func (r *receiptAwarePreparingRunner) Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	return mcpplatform.ExecutionResult{}, errors.New("unprepared execution path used")
}

func (r *receiptAwarePreparingRunner) PrepareExecution(_ context.Context, request mcpplatform.ExecutionRequest) (PreparedExecution, error) {
	return &receiptAwarePreparedExecution{runner: r, executionID: request.ExecutionID}, nil
}

type receiptAwarePreparedExecution struct {
	runner      *receiptAwarePreparingRunner
	executionID string
}

func (*receiptAwarePreparedExecution) JobID() string { return "sandbox-job-123" }

func (p *receiptAwarePreparedExecution) Execute(ctx context.Context) (mcpplatform.ExecutionResult, error) {
	receipt, err := p.runner.store.Get(ctx, p.executionID)
	if err != nil {
		return mcpplatform.ExecutionResult{}, err
	}
	if receipt.JobID != p.JobID() {
		return mcpplatform.ExecutionResult{}, fmt.Errorf("side effect started before Job ID was durable")
	}
	p.runner.executed.Store(true)
	return mcpplatform.ExecutionResult{JobID: p.JobID(), Output: map[string]any{"ok": true}}, nil
}

func (*receiptAwarePreparedExecution) Close(context.Context) error { return nil }

func TestOCIRequestsNeverFallBackToSharedRunner(t *testing.T) {
	shared := &fakeRunner{}
	service := NewService(shared, fixedScanner{status: "passed"})
	_, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{
			ImageRef: "registry.example.com/tool@sha256:" + strings.Repeat("a", 64),
		},
	})
	if !errors.Is(err, ErrImageRejected) {
		t.Fatalf("expected isolated OCI runner rejection, got %v", err)
	}
	if shared.validations != 0 {
		t.Fatal("OCI validation reached the shared HTTPS Runner")
	}
}

func TestPreparedExecutionPersistsJobIDBeforeSideEffect(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-prepared-job.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	store := NewReceiptStore(db)
	runner := &receiptAwarePreparingRunner{store: store}
	service := NewService(&fakeRunner{}, fixedScanner{status: "passed"}).
		WithOCIRunner(runner).
		WithReceiptStore(store)

	receipt, err := service.Execute(context.Background(), receiptExecutionRequest("wrap-token"))
	if err != nil {
		t.Fatal(err)
	}
	if !runner.executed.Load() || receipt.JobID != "sandbox-job-123" || receipt.Status != models.SandboxExecutionStatusSucceeded {
		t.Fatalf("prepared execution was not durable before side effect: %#v", receipt)
	}
}

func TestRejectsPrivateHTTPSDestination(t *testing.T) {
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "passed"})
	_, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceHTTPS,
		Definition: mcpplatform.InstallationDefinition{
			Transport:        "streamable_http",
			EndpointURL:      "https://127.0.0.1/mcp",
			NetworkAllowlist: []string{"127.0.0.1"},
		},
	})
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("expected private address rejection, got %v", err)
	}
	if runner.validations != 0 {
		t.Fatal("private endpoint reached runner")
	}
}

func TestInterviewHostAllowsOnlyExactConfiguredPrivateDestination(t *testing.T) {
	t.Setenv("APP_ENV", "interview")
	t.Setenv("MCP_INTERVIEW_TRUSTED_HOSTS", "interview-mcp")
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "passed"})
	service.resolver = fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("172.20.0.10")}}}
	request := mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceHTTPS,
		Definition: mcpplatform.InstallationDefinition{
			Transport:        "streamable_http",
			EndpointURL:      "https://interview-mcp:8443/mcp",
			NetworkAllowlist: []string{"interview-mcp"},
		},
	}
	if _, err := service.Validate(context.Background(), request); err != nil {
		t.Fatalf("expected exact interview host to be accepted: %v", err)
	}
	if runner.validations != 1 {
		t.Fatalf("expected runner validation, got %d", runner.validations)
	}

	request.Definition.NetworkAllowlist = []string{"*.local"}
	if _, err := service.Validate(context.Background(), request); err == nil {
		t.Fatal("expected wildcard interview allowlist rejection")
	}
}

func TestUnsafeIPRejectsSpecialUseNetworks(t *testing.T) {
	for _, address := range []string{
		"0.0.0.1",
		"100.64.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"64:ff9b:1::1",
		"2001:db8::1",
		"3fff::1",
	} {
		if !unsafeIP(net.ParseIP(address)) {
			t.Errorf("special-use address %s was accepted", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if unsafeIP(net.ParseIP(address)) {
			t.Errorf("public address %s was rejected", address)
		}
	}
}

func TestCriticalImageIsQuarantinedBeforeRunner(t *testing.T) {
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "critical"})
	result, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{
			Transport: "stdio",
			ImageRef:  "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ScanStatus != "critical" || runner.validations != 0 {
		t.Fatalf("critical image was not quarantined: status=%q runner_calls=%d", result.ScanStatus, runner.validations)
	}
}

func TestRejectsMutableImageTag(t *testing.T) {
	service := newTestService(&fakeRunner{}, fixedScanner{status: "passed"})
	_, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{Transport: "stdio", ImageRef: "registry.example.com/tool:latest"},
	})
	if !errors.Is(err, ErrImageRejected) {
		t.Fatalf("expected mutable image rejection, got %v", err)
	}
}

func TestReadRiskRequiresInstallerDeclarationAndRunnerClassification(t *testing.T) {
	image := "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, testCase := range []struct {
		name       string
		config     map[string]any
		wantVerify bool
	}{
		{name: "undeclared", config: map[string]any{}, wantVerify: false},
		{name: "declared", config: map[string]any{"read_tools": []any{"search"}}, wantVerify: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := newTestService(&fakeRunner{}, fixedScanner{status: "passed"})
			result, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
				SourceType: models.MCPInstallationSourceOCI,
				Definition: mcpplatform.InstallationDefinition{
					Transport: "stdio",
					ImageRef:  image,
					Config:    testCase.config,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Tools) != 1 || result.Tools[0].RiskVerified != testCase.wantVerify {
				t.Fatalf("unexpected verified risk: %#v", result.Tools)
			}
		})
	}
}

func TestExecutionReceiptReplaysAcrossRestartAndExcludesWrapToken(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-replay.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	store := NewReceiptStore(db)
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(store)
	request := receiptExecutionRequest("wrap-token-first")

	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	if first.Status != models.SandboxExecutionStatusSucceeded || first.Output["ok"] != true {
		t.Fatalf("unexpected first receipt: %#v", first)
	}
	request.SecretWrapToken = "wrap-token-rotated"
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("replayed execution: %v", err)
	}
	if second.RequestDigest != first.RequestDigest || runner.executionCount() != 1 {
		t.Fatalf("token rotation changed replay identity: first=%#v second=%#v calls=%d", first, second, runner.executionCount())
	}

	restarted := newTestService(&fakeRunner{}, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	stored, err := restarted.LookupExecution(context.Background(), request.ExecutionID)
	if err != nil || stored.Status != models.SandboxExecutionStatusSucceeded || stored.Output["ok"] != true {
		t.Fatalf("lookup after restart: receipt=%#v err=%v", stored, err)
	}
	var row models.SandboxExecutionReceipt
	if err := db.Where("execution_id = ?", request.ExecutionID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	persisted := fmt.Sprintf("%+v", row)
	if containsAny(persisted, "wrap-token-first", "wrap-token-rotated") {
		t.Fatalf("secret wrapping token was persisted: %s", persisted)
	}
}

func TestConcurrentExecutionReceiptHasSingleRunnerWinner(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-concurrent.db")
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runner := &fakeRunner{execute: func(_ context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
		once.Do(func() { close(started) })
		<-release
		return mcpplatform.ExecutionResult{ExecutionID: request.ExecutionID, JobID: request.ExecutionID, Output: map[string]any{"winner": true}}, nil
	}}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	request := receiptExecutionRequest("wrap-token")

	const concurrency = 50
	errorsByCall := make(chan error, concurrency)
	var wait sync.WaitGroup
	wait.Add(concurrency)
	for range concurrency {
		go func() {
			defer wait.Done()
			_, executeErr := service.Execute(context.Background(), request)
			if executeErr != nil && !errors.Is(executeErr, ErrExecutionInProgress) {
				errorsByCall <- executeErr
			}
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(errorsByCall)
	for executeErr := range errorsByCall {
		t.Fatalf("concurrent execution failed: %v", executeErr)
	}
	if runner.executionCount() != 1 {
		t.Fatalf("Runner invoked %d times, want 1", runner.executionCount())
	}
	receipt, err := service.LookupExecution(context.Background(), request.ExecutionID)
	if err != nil || receipt.Status != models.SandboxExecutionStatusSucceeded {
		t.Fatalf("terminal receipt missing: receipt=%#v err=%v", receipt, err)
	}
}

func TestExecutionReceiptRejectsDigestCollision(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-conflict.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	request := receiptExecutionRequest("wrap-token")
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Arguments = map[string]any{"query": "different"}
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrExecutionConflict) {
		t.Fatalf("expected execution conflict, got %v", err)
	}
	if runner.executionCount() != 1 {
		t.Fatalf("digest collision reached Runner %d times", runner.executionCount())
	}
}

func TestTerminalReceiptPersistsAfterCallerCancellation(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-cancel.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeRunner{execute: func(executionCtx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
		cancel()
		select {
		case <-executionCtx.Done():
			return mcpplatform.ExecutionResult{}, fmt.Errorf("caller cancellation reached Runner: %w", executionCtx.Err())
		default:
		}
		return mcpplatform.ExecutionResult{ExecutionID: request.ExecutionID, JobID: request.ExecutionID, Output: map[string]any{"persisted": true}}, nil
	}}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	request := receiptExecutionRequest("wrap-token")
	receipt, err := service.Execute(ctx, request)
	if err != nil || receipt.Status != models.SandboxExecutionStatusSucceeded {
		t.Fatalf("execute with canceled caller: receipt=%#v err=%v", receipt, err)
	}
	stored, err := service.LookupExecution(context.Background(), request.ExecutionID)
	if err != nil || stored.Output["persisted"] != true {
		t.Fatalf("detached terminal write was not durable: receipt=%#v err=%v", stored, err)
	}
}

func TestTerminalReceiptRedactsSecretWrapTokenFromErrors(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-redaction.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	const token = "one-time-sensitive-wrap-token"
	runner := &fakeRunner{execute: func(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
		return mcpplatform.ExecutionResult{}, fmt.Errorf("unwrap failed for token %s", token)
	}}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	receipt, err := service.Execute(context.Background(), receiptExecutionRequest(token))
	if err != nil {
		t.Fatalf("persist failed terminal receipt: %v", err)
	}
	if receipt.Status != models.SandboxExecutionStatusFailed || !strings.Contains(receipt.ErrorMessage, "[REDACTED]") {
		t.Fatalf("unexpected redacted receipt: %#v", receipt)
	}
	var row models.SandboxExecutionReceipt
	if err := db.Where("execution_id = ?", receipt.ExecutionID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%+v", row), token) {
		t.Fatal("secret wrapping token was persisted in a failed receipt")
	}
}

func TestTerminalReceiptRetriesTransientDatabaseFailure(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-terminal-retry.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	const callbackName = "sandbox:test_transient_terminal_failure"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if attempts.Add(1) <= 2 {
			tx.AddError(errors.New("transient receipt database failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(callbackName)

	service := newTestService(&fakeRunner{}, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	receipt, err := service.Execute(context.Background(), receiptExecutionRequest("wrap-token"))
	if err != nil {
		t.Fatalf("terminal retry failed: %v", err)
	}
	if receipt.Status != models.SandboxExecutionStatusSucceeded || attempts.Load() != 3 {
		t.Fatalf("unexpected retry result: receipt=%#v attempts=%d", receipt, attempts.Load())
	}
}

func TestStaleRunningReceiptBecomesOutcomeUnknownWithoutReplay(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-stale.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	store := NewReceiptStore(db)
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(store)
	request := normalizeExecutionRequest(receiptExecutionRequest("wrap-token"))
	digest, err := executionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, winner, err := store.Acquire(context.Background(), models.SandboxExecutionReceipt{
		ExecutionID:    request.ExecutionID,
		RequestDigest:  digest,
		OrganizationID: request.OrganizationID,
		UserID:         request.UserID,
		ConversationID: request.ConversationID,
		RunID:          request.RunID,
		RunRef:         request.RunRef,
		ToolCallID:     request.ToolCallID,
		InstallationID: request.InstallationID,
		RevisionID:     request.RevisionID,
		ToolID:         request.ToolID,
		ToolName:       request.ToolName,
		SourceType:     request.SourceType,
		Status:         models.SandboxExecutionStatusRunning,
		TimeoutMS:      request.TimeoutMS,
		StartedAt:      now.Add(-time.Minute),
		StaleAt:        now.Add(-time.Second),
		ExpiresAt:      now.Add(24 * time.Hour),
	})
	if err != nil || !winner {
		t.Fatalf("seed running receipt: winner=%v err=%v", winner, err)
	}
	receipt, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("reconcile stale receipt: %v", err)
	}
	if receipt.Status != models.SandboxExecutionStatusOutcomeUnknown || receipt.ErrorCode != "SANDBOX_OUTCOME_UNKNOWN" {
		t.Fatalf("unexpected stale receipt: %#v", receipt)
	}
	if runner.executionCount() != 0 {
		t.Fatalf("stale unknown receipt replayed Runner %d times", runner.executionCount())
	}
}

func TestReceiptInsertFailureDoesNotInvokeRunner(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-missing-table.db")
	runner := &fakeRunner{}
	service := newTestService(runner, fixedScanner{status: "passed"}).WithReceiptStore(NewReceiptStore(db))
	if _, err := service.Execute(context.Background(), receiptExecutionRequest("wrap-token")); !errors.Is(err, ErrReceiptUnavailable) {
		t.Fatalf("expected receipt dependency failure, got %v", err)
	}
	if runner.executionCount() != 0 {
		t.Fatalf("Runner invoked after receipt insert failure %d times", runner.executionCount())
	}
}

func TestReceiptStoreDeletesOnlyExpiredRows(t *testing.T) {
	db := testutil.OpenSQLite(t, "sandbox-receipt-cleanup.db")
	if err := db.AutoMigrate(&models.SandboxExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, row := range []models.SandboxExecutionReceipt{
		minimalReceipt("expired", now.Add(-time.Minute)),
		minimalReceipt("retained", now.Add(time.Minute)),
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := NewReceiptStore(db).DeleteExpired(context.Background(), now, 500)
	if err != nil || deleted != 1 {
		t.Fatalf("delete expired receipts: deleted=%d err=%v", deleted, err)
	}
	if _, err := NewReceiptStore(db).Get(context.Background(), "expired"); !errors.Is(err, ErrReceiptNotFound) {
		t.Fatalf("expired receipt remained: %v", err)
	}
	if _, err := NewReceiptStore(db).Get(context.Background(), "retained"); err != nil {
		t.Fatalf("unexpired receipt was deleted: %v", err)
	}
}

func receiptExecutionRequest(secretWrapToken string) mcpplatform.ExecutionRequest {
	return mcpplatform.ExecutionRequest{
		ExecutionID:    "mcp:receipt-test",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		RunRef:         "agent:99",
		ToolCallID:     "call-1",
		InstallationID: 11,
		RevisionID:     12,
		ToolID:         13,
		SourceType:     models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{
			Transport: "stdio",
			ImageRef:  "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		ToolName:        "search",
		Arguments:       map[string]any{"query": "security"},
		SecretWrapToken: secretWrapToken,
		TimeoutMS:       30_000,
		OutputLimit:     mcpplatform.DefaultOutputLimit,
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func minimalReceipt(executionID string, expiresAt time.Time) models.SandboxExecutionReceipt {
	now := time.Now().UTC()
	return models.SandboxExecutionReceipt{
		ExecutionID:    executionID,
		RequestDigest:  strings.Repeat("a", 64),
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		RunRef:         "agent:99",
		ToolCallID:     "call-1",
		InstallationID: 11,
		RevisionID:     12,
		ToolID:         13,
		ToolName:       "search",
		SourceType:     models.MCPInstallationSourceOCI,
		Status:         models.SandboxExecutionStatusSucceeded,
		TimeoutMS:      30_000,
		StartedAt:      now,
		StaleAt:        now.Add(time.Minute),
		CompletedAt:    &now,
		ExpiresAt:      expiresAt,
	}
}
