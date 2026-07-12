package mcpplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type fakeSandbox struct {
	mu          sync.Mutex
	validations int
	executions  int
	lookups     int
	tools       []DiscoveredTool
	validate    func(context.Context, ValidationRequest) (ValidationResult, error)
	execute     func(context.Context, ExecutionRequest) (ExecutionResult, error)
	lookup      func(context.Context, string) (SandboxExecutionReceipt, error)
	receipts    map[string]SandboxExecutionReceipt
}

func (f *fakeSandbox) Validate(ctx context.Context, request ValidationRequest) (ValidationResult, error) {
	f.mu.Lock()
	f.validations++
	validate := f.validate
	f.mu.Unlock()
	if validate != nil {
		return validate(ctx, request)
	}
	return ValidationResult{ScanStatus: "passed", Tools: f.tools}, nil
}

func (f *fakeSandbox) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	f.mu.Lock()
	f.executions++
	if f.receipts == nil {
		f.receipts = make(map[string]SandboxExecutionReceipt)
	}
	started := time.Now().UTC()
	f.receipts[request.ExecutionID] = fakeSandboxReceipt(request, ExecutionResult{
		Status:    SandboxExecutionStatusRunning,
		StartedAt: &started,
	})
	execute := f.execute
	f.mu.Unlock()
	var result ExecutionResult
	var err error
	if execute != nil {
		result, err = execute(ctx, request)
	} else {
		result = ExecutionResult{JobID: "job-1", Output: map[string]any{"ok": true}}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		result = fakeSandboxReceipt(request, result)
		if result.Status == "" {
			result.Status = SandboxExecutionStatusSucceeded
		}
		completed := time.Now().UTC()
		if result.CompletedAt == nil && isSandboxReceiptTerminal(result.Status) {
			result.CompletedAt = &completed
		}
		f.receipts[request.ExecutionID] = result
	} else if errors.Is(err, context.DeadlineExceeded) {
		receipt := f.receipts[request.ExecutionID]
		receipt.Status = SandboxExecutionStatusTimedOut
		receipt.ErrorCode = "sandbox_timeout"
		receipt.ErrorMessage = err.Error()
		completed := time.Now().UTC()
		receipt.CompletedAt = &completed
		f.receipts[request.ExecutionID] = receipt
	}
	return result, err
}

func (f *fakeSandbox) LookupExecution(ctx context.Context, executionID string) (SandboxExecutionReceipt, error) {
	f.mu.Lock()
	f.lookups++
	lookup := f.lookup
	receipt, ok := f.receipts[executionID]
	f.mu.Unlock()
	if lookup != nil {
		return lookup(ctx, executionID)
	}
	if !ok {
		return SandboxExecutionReceipt{}, ErrSandboxExecutionNotFound
	}
	return receipt, nil
}

func (f *fakeSandbox) updateReceipt(executionID string, update func(*SandboxExecutionReceipt)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	receipt := f.receipts[executionID]
	update(&receipt)
	f.receipts[executionID] = receipt
}

func fakeSandboxReceipt(request ExecutionRequest, receipt SandboxExecutionReceipt) SandboxExecutionReceipt {
	if receipt.ExecutionID == "" {
		receipt.ExecutionID = request.ExecutionID
	}
	if receipt.RequestDigest == "" {
		receipt.RequestDigest, _ = ExecutionRequestDigest(request)
	}
	if receipt.JobID == "" {
		receipt.JobID = request.ExecutionID
	}
	receipt.OrganizationID = request.OrganizationID
	receipt.UserID = request.UserID
	receipt.ConversationID = request.ConversationID
	receipt.RunID = request.RunID
	receipt.RunRef = request.RunRef
	receipt.ToolCallID = request.ToolCallID
	receipt.InstallationID = request.InstallationID
	receipt.RevisionID = request.RevisionID
	receipt.ToolID = request.ToolID
	receipt.ToolName = request.ToolName
	return receipt
}

func isSandboxReceiptTerminal(status string) bool {
	switch status {
	case SandboxExecutionStatusSucceeded,
		SandboxExecutionStatusFailed,
		SandboxExecutionStatusTimedOut,
		SandboxExecutionStatusCanceled,
		SandboxExecutionStatusOutcomeUnknown:
		return true
	default:
		return false
	}
}

func setupService(t *testing.T) (*gorm.DB, *Service, *fakeSandbox) {
	t.Helper()
	db := testutil.OpenSQLite(t, "mcp.db")
	if err := db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
		&models.OrganizationAuditEvent{},
		&models.MCPInstallation{},
		&models.MCPInstallationRevision{},
		&models.MCPTool{},
		&models.MCPExecution{},
		&models.AgentSkill{},
		&models.AgentSkillTool{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("migrate MCP models: %v", err)
	}
	org := models.Organization{ID: 1, Name: "Acme", Slug: "acme", CreatedBy: 7}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	now := time.Now().UTC()
	members := []models.OrganizationMember{
		{OrganizationID: 1, UserID: 7, Role: models.OrganizationRoleOwner, JoinedAt: now},
		{OrganizationID: 1, UserID: 8, Role: models.OrganizationRoleMember, JoinedAt: now},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}
	sandbox := &fakeSandbox{tools: []DiscoveredTool{{
		Name:         "search",
		RiskVerified: true,
		InputSchema: map[string]any{
			"type": "object",
		},
		Risk: models.MCPToolRiskRead,
	}}}
	return db, NewService(db, nil).WithSandbox(sandbox).WithOutbox(events.NewStore(db)), sandbox
}

func setupActiveTool(t *testing.T) (*gorm.DB, *Service, *fakeSandbox) {
	t.Helper()
	db, service, sandbox := setupService(t)
	installation, err := service.CreateInstallation(context.Background(), 1, 7, ociInstallation())
	if err != nil {
		t.Fatalf("create installation: %v", err)
	}
	if _, err := service.ValidateInstallation(context.Background(), 1, 7, installation.ID); err != nil {
		t.Fatalf("validate installation: %v", err)
	}
	if _, err := service.ActivateInstallation(context.Background(), 1, 7, installation.ID); err != nil {
		t.Fatalf("activate installation: %v", err)
	}
	return db, service, sandbox
}

func TestValidationFailsClosedForUnrecognizedScanStatus(t *testing.T) {
	for _, scanStatus := range []string{"", "unknown"} {
		t.Run(scanStatus, func(t *testing.T) {
			ctx := context.Background()
			db, service, sandbox := setupService(t)
			sandbox.validate = func(context.Context, ValidationRequest) (ValidationResult, error) {
				return ValidationResult{ScanStatus: scanStatus, Tools: sandbox.tools}, nil
			}
			installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("expected invalid scan result to fail closed, got %v", err)
			}
			if err := db.Take(installation, installation.ID).Error; err != nil {
				t.Fatal(err)
			}
			if installation.Status != models.MCPInstallationStatusFailed {
				t.Fatalf("expected failed installation, got %q", installation.Status)
			}
			if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("unverified revision activated: %v", err)
			}
		})
	}
}

func TestUnverifiedReadRiskDefaultsToUnknown(t *testing.T) {
	ctx := context.Background()
	db, service, sandbox := setupService(t)
	sandbox.tools[0].RiskVerified = false
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	var tool models.MCPTool
	if err := db.Where("installation_id = ?", installation.ID).Take(&tool).Error; err != nil {
		t.Fatal(err)
	}
	if tool.Risk != models.MCPToolRiskUnknown {
		t.Fatalf("expected unverified read risk to become unknown, got %q", tool.Risk)
	}
	if _, err := service.Execute(ctx, ExecuteInput{
		ExecutionID:    "unverified-risk",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		ToolCallID:     "unverified-call",
		ToolName:       tool.NamespacedName,
	}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected unknown tool to require approval, got %v", err)
	}
	if sandbox.executions != 0 {
		t.Fatal("unverified read tool reached sandbox execution")
	}
}

func ociInstallation() CreateInstallationInput {
	return CreateInstallationInput{
		Scope:       models.MCPInstallationScopePersonal,
		DisplayName: "Search MCP",
		SourceType:  models.MCPInstallationSourceOCI,
		InstallationDefinition: InstallationDefinition{
			Transport: "stdio",
			ImageRef:  "registry.example.com/search@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
}

func TestInstallationLifecycleAndPersonalVisibility(t *testing.T) {
	ctx := context.Background()
	_, service, _ := setupService(t)
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatalf("create installation: %v", err)
	}
	items, err := service.ListInstallations(ctx, 1, 8)
	if err != nil {
		t.Fatalf("list installations: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("personal installation leaked to another member: %#v", items)
	}
	if _, _, err := service.GetInstallation(ctx, 1, 8, installation.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected hidden installation to return not found, got %v", err)
	}
	validated, err := service.ValidateInstallation(ctx, 1, 7, installation.ID)
	if err != nil {
		t.Fatalf("validate installation: %v", err)
	}
	if validated.Status != models.MCPInstallationStatusDisabled {
		t.Fatalf("expected validated installation to be disabled, got %q", validated.Status)
	}
	active, err := service.ActivateInstallation(ctx, 1, 7, installation.ID)
	if err != nil {
		t.Fatalf("activate installation: %v", err)
	}
	if active.Status != models.MCPInstallationStatusActive || active.ActiveRevisionID == nil {
		t.Fatalf("installation did not activate: %#v", active)
	}
	published, err := service.PublishInstallation(ctx, 1, 7, installation.ID)
	if err != nil {
		t.Fatalf("publish installation: %v", err)
	}
	if published.Scope != models.MCPInstallationScopeOrganization {
		t.Fatalf("expected organization scope, got %q", published.Scope)
	}
	items, err = service.ListInstallations(ctx, 1, 8)
	if err != nil || len(items) != 1 {
		t.Fatalf("published installation not visible to member: items=%d err=%v", len(items), err)
	}
}

func TestOrganizationSkillRequiresOrganizationToolsAndAppearsInCatalog(t *testing.T) {
	ctx := context.Background()
	_, service, _ := setupService(t)
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	tools, err := service.ListTools(ctx, 1, 7, installation.ID)
	if err != nil || len(tools) != 1 {
		t.Fatalf("list tools: tools=%#v err=%v", tools, err)
	}
	input := CreateSkillInput{
		Scope:        models.MCPInstallationScopeOrganization,
		Name:         "Search policy",
		Instructions: "Use the search tool only for policy lookups.",
		ToolIDs:      []uint64{tools[0].ID},
	}
	if _, err := service.CreateSkill(ctx, 1, 7, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected organization skill to reject personal tool, got %v", err)
	}
	if _, err := service.PublishInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatalf("publish installation: %v", err)
	}
	skill, err := service.CreateSkill(ctx, 1, 7, input)
	if err != nil {
		t.Fatalf("create organization skill: %v", err)
	}
	catalog, err := service.CatalogSkills(ctx, 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 || catalog[0].ID != skill.ID || len(catalog[0].ToolNames) != 1 || catalog[0].ToolNames[0] != tools[0].NamespacedName {
		t.Fatalf("unexpected skill catalog: %#v", catalog)
	}
}

func TestExecuteIsIdempotentAndWriteToolsRequireApproval(t *testing.T) {
	ctx := context.Background()
	_, service, sandbox := setupService(t)
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	input := ExecuteInput{
		ExecutionID:    "execution-1",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		ToolCallID:     "call-1",
		ToolName:       "mcp.1.search",
		Arguments:      map[string]any{"query": "security"},
	}
	first, err := service.Execute(ctx, input)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	second, err := service.Execute(ctx, input)
	if err != nil {
		t.Fatalf("repeat execution: %v", err)
	}
	if first.ID != second.ID || sandbox.executions != 1 {
		t.Fatalf("execution was not idempotent: ids=%d/%d calls=%d", first.ID, second.ID, sandbox.executions)
	}
	collision := input
	collision.ToolCallID = "different-call"
	if _, err := service.Execute(ctx, collision); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected execution fingerprint mismatch to be forbidden, got %v", err)
	}
	if sandbox.executions != 1 {
		t.Fatalf("execution id collision reached sandbox")
	}

	var tool models.MCPTool
	if err := service.db.Where("namespaced_name = ?", "mcp.1.search").Take(&tool).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.db.Model(&tool).Update("risk", models.MCPToolRiskWrite).Error; err != nil {
		t.Fatal(err)
	}
	input.ExecutionID = "execution-2"
	input.ToolCallID = "call-2"
	if _, err := service.Execute(ctx, input); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected approval requirement, got %v", err)
	}
	if sandbox.executions != 1 {
		t.Fatalf("write tool reached sandbox without approval")
	}
}

func TestDuplicateRunningExecutionDoesNotReturnEmptySuccess(t *testing.T) {
	ctx := context.Background()
	_, service, sandbox := setupService(t)
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		close(started)
		<-release
		return ExecutionResult{JobID: "job-blocked", Output: map[string]any{"ok": true}}, nil
	}
	input := ExecuteInput{
		ExecutionID:    "execution-running",
		RunRef:         "agent:99",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		ToolCallID:     "call-running",
		ToolName:       "mcp.1.search",
	}
	firstDone := make(chan error, 1)
	go func() {
		_, executeErr := service.Execute(ctx, input)
		firstDone <- executeErr
	}()
	<-started

	existing, err := service.Execute(ctx, input)
	if !errors.Is(err, ErrExecutionInProgress) || existing == nil || existing.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("expected running execution, got execution=%#v err=%v", existing, err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if sandbox.executions != 1 {
		t.Fatalf("duplicate running execution reached sandbox %d times", sandbox.executions)
	}
}

func TestExecuteApprovedRejectsPinnedRevisionDriftBeforeSandbox(t *testing.T) {
	ctx := context.Background()
	db, service, sandbox := setupService(t)
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	var pinned models.MCPTool
	if err := db.Where("installation_id = ?", installation.ID).Take(&pinned).Error; err != nil {
		t.Fatal(err)
	}
	definition := InstallationDefinition{
		Transport: "stdio",
		ImageRef:  "registry.example.com/search@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := service.UpdateInstallation(ctx, 1, 7, installation.ID, UpdateInstallationInput{Definition: &definition}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}

	_, err = service.ExecuteApproved(ctx, ExecuteInput{
		ExecutionID:            "execution-pinned-r1",
		RunRef:                 "agent:99",
		OrganizationID:         1,
		UserID:                 7,
		ConversationID:         42,
		RunID:                  99,
		ToolCallID:             "call-pinned-r1",
		ToolName:               pinned.NamespacedName,
		ExpectedInstallationID: pinned.InstallationID,
		ExpectedRevisionID:     pinned.RevisionID,
		ExpectedToolID:         pinned.ID,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected revision drift to be forbidden, got %v", err)
	}
	if sandbox.executions != 0 {
		t.Fatalf("revision drift reached sandbox %d times", sandbox.executions)
	}
	var executions int64
	if err := db.Model(&models.MCPExecution{}).Where("execution_id = ?", "execution-pinned-r1").Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if executions != 0 {
		t.Fatalf("revision drift created %d execution rows", executions)
	}
}

func TestExecuteValidatesDiscoveredInputSchema(t *testing.T) {
	ctx := context.Background()
	_, service, sandbox := setupService(t)
	sandbox.tools[0].InputSchema = map[string]any{
		"type":                 "object",
		"required":             []any{"query"},
		"additionalProperties": false,
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
		t.Fatal(err)
	}
	input := ExecuteInput{
		ExecutionID:    "schema-validation",
		RunRef:         "agent:99",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		ToolCallID:     "schema-call",
		ToolName:       "mcp.1.search",
	}
	if _, err := service.Execute(ctx, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected missing required argument to fail, got %v", err)
	}
	input.Arguments = map[string]any{"query": "security"}
	if _, err := service.Execute(ctx, input); err != nil {
		t.Fatalf("expected valid arguments to execute, got %v", err)
	}
}

func TestOCIImageMustBeDigestPinned(t *testing.T) {
	_, service, _ := setupService(t)
	input := ociInstallation()
	input.ImageRef = "registry.example.com/search:latest"
	if _, err := service.CreateInstallation(context.Background(), 1, 7, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestInstallationDefinitionCannotPersistSecretValues(t *testing.T) {
	_, service, _ := setupService(t)
	input := ociInstallation()
	input.Config = map[string]any{"api_key": "plaintext-secret"}
	if _, err := service.CreateInstallation(context.Background(), 1, 7, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected secret-like config to be rejected, got %v", err)
	}

	input = ociInstallation()
	input.Config = map[string]any{
		"read_tools": []any{"search"},
		"secret_env": map[string]any{"SEARCH_API_KEY": "search_api_key"},
	}
	if _, err := service.CreateInstallation(context.Background(), 1, 7, input); err != nil {
		t.Fatalf("expected secret reference mapping to be accepted, got %v", err)
	}
}

func TestHTTPSEndpointCannotPersistCredentialQuery(t *testing.T) {
	_, service, _ := setupService(t)
	input := CreateInstallationInput{
		DisplayName: "HTTPS MCP",
		SourceType:  models.MCPInstallationSourceHTTPS,
		InstallationDefinition: InstallationDefinition{
			Transport:   "streamable_http",
			EndpointURL: "https://mcp.example.com/tools?token=plaintext",
		},
	}
	if _, err := service.CreateInstallation(context.Background(), 1, 7, input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected credential query to be rejected, got %v", err)
	}
}

func TestExecutionEnforcesTimeoutAndOutputLimit(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		execution  func(context.Context, ExecutionRequest) (ExecutionResult, error)
		wantError  error
		wantStatus string
	}{
		{
			name: "timeout",
			execution: func(ctx context.Context, _ ExecutionRequest) (ExecutionResult, error) {
				<-ctx.Done()
				return ExecutionResult{}, ctx.Err()
			},
			wantError:  context.DeadlineExceeded,
			wantStatus: models.MCPExecutionStatusTimedOut,
		},
		{
			name: "oversized output",
			execution: func(context.Context, ExecutionRequest) (ExecutionResult, error) {
				return ExecutionResult{Output: map[string]any{"value": strings.Repeat("x", DefaultOutputLimit)}}, nil
			},
			wantError:  ErrOutputTooLarge,
			wantStatus: models.MCPExecutionStatusFailed,
		},
		{
			name: "outcome unknown",
			execution: func(context.Context, ExecutionRequest) (ExecutionResult, error) {
				return ExecutionResult{
					Status:       SandboxExecutionStatusOutcomeUnknown,
					ErrorCode:    "SANDBOX_OUTCOME_UNKNOWN",
					ErrorMessage: "sandbox stopped before persisting a result",
				}, nil
			},
			wantError:  ErrExecutionTerminal,
			wantStatus: models.MCPExecutionStatusTimedOut,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			_, service, sandbox := setupService(t)
			installation, err := service.CreateInstallation(ctx, 1, 7, ociInstallation())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ValidateInstallation(ctx, 1, 7, installation.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := service.ActivateInstallation(ctx, 1, 7, installation.ID); err != nil {
				t.Fatal(err)
			}
			sandbox.execute = testCase.execution
			service.executionTimeout = 5 * time.Millisecond
			executionID := "execution-" + strings.ReplaceAll(testCase.name, " ", "-")
			input := ExecuteInput{
				ExecutionID:    executionID,
				OrganizationID: 1,
				UserID:         7,
				ConversationID: 42,
				RunID:          99,
				ToolCallID:     "call-1",
				ToolName:       "mcp.1.search",
			}
			_, err = service.Execute(ctx, input)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("expected %v, got %v", testCase.wantError, err)
			}
			var stored models.MCPExecution
			if err := service.db.Where("execution_id = ?", executionID).Take(&stored).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Status != testCase.wantStatus {
				t.Fatalf("expected status %q, got %q", testCase.wantStatus, stored.Status)
			}
			existing, retryErr := service.Execute(ctx, input)
			if !errors.Is(retryErr, ErrExecutionTerminal) || existing == nil || existing.Status != testCase.wantStatus {
				t.Fatalf("expected terminal retry result, got execution=%#v err=%v", existing, retryErr)
			}
			if sandbox.executions != 1 {
				t.Fatalf("terminal execution was replayed %d times", sandbox.executions)
			}
		})
	}
}

func TestExecutionCreationConflictLazilyReconcilesLateTerminalReceipt(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("connection reset after sandbox accepted execution")
	}
	input := ExecuteInput{
		ExecutionID:    "execution-late-terminal",
		RunRef:         "agent:99",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          99,
		ToolCallID:     "call-late-terminal",
		ToolName:       "mcp.1.search",
	}
	first, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrExecutionInProgress) || first.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("expected ambiguous execution to remain running, execution=%+v err=%v", first, err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.Status = SandboxExecutionStatusSucceeded
		receipt.Output = map[string]any{"recovered": true}
		completed := time.Now().UTC()
		receipt.CompletedAt = &completed
	})

	recovered, err := service.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("lazy reconciliation failed: %v", err)
	}
	if recovered.Status != models.MCPExecutionStatusSucceeded || recovered.NextReconcileAt != nil || !strings.Contains(recovered.OutputJSON, "recovered") {
		t.Fatalf("unexpected recovered execution: %+v", recovered)
	}
	if sandbox.executions != 1 {
		t.Fatalf("late reconciliation replayed sandbox side effect %d times", sandbox.executions)
	}
	var terminalEvents int64
	if err := db.Model(&models.EventOutbox{}).
		Where("event = ? AND idempotency_key = ?", EventMCPExecutionTerminal, EventMCPExecutionTerminal+":"+input.ExecutionID).
		Count(&terminalEvents).Error; err != nil {
		t.Fatal(err)
	}
	if terminalEvents != 1 {
		t.Fatalf("expected one durable terminal event, got %d", terminalEvents)
	}
}

func TestGetExecutionLazilyReconcilesActiveReceipt(t *testing.T) {
	_, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("response was lost")
	}
	input := ExecuteInput{
		ExecutionID:    "execution-get-reconcile",
		RunRef:         "run:101",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          101,
		ToolCallID:     "call-get-reconcile",
		ToolName:       "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("expected ambiguous execution, got %v", err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.Status = SandboxExecutionStatusSucceeded
		receipt.Output = map[string]any{"source": "receipt"}
	})

	execution, err := service.GetExecution(context.Background(), 1, 7, input.ExecutionID)
	if err != nil {
		t.Fatalf("get execution reconciliation: %v", err)
	}
	if execution.Status != models.MCPExecutionStatusSucceeded || sandbox.executions != 1 {
		t.Fatalf("get did not reconcile without replay: execution=%+v sandbox_calls=%d", execution, sandbox.executions)
	}
}

func TestRunningReceiptWithoutRunnerJobIDRemainsInProgress(t *testing.T) {
	_, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("sandbox request still running")
	}
	input := ExecuteInput{
		ExecutionID: "execution-running-no-job", RunRef: "run:102", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 102, ToolCallID: "call-running-no-job", ToolName: "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("expected ambiguous execution, got %v", err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.JobID = ""
		receipt.Status = SandboxExecutionStatusRunning
	})

	execution, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrExecutionInProgress) || execution.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("running receipt was treated as invalid: execution=%+v err=%v", execution, err)
	}
}

func TestMissingReceiptOnlyTimesOutAfterRecoveryDeadline(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("connection closed before receipt response")
	}
	sandbox.lookup = func(context.Context, string) (SandboxExecutionReceipt, error) {
		return SandboxExecutionReceipt{}, ErrSandboxExecutionNotFound
	}
	input := ExecuteInput{
		ExecutionID: "execution-missing-receipt", RunRef: "run:103", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 103, ToolCallID: "call-missing-receipt", ToolName: "mcp.1.search",
	}
	fresh, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrExecutionInProgress) || fresh.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("fresh missing receipt should remain active: execution=%+v err=%v", fresh, err)
	}
	if fresh.ReconcileAttempts != 1 || fresh.NextReconcileAt == nil || !fresh.NextReconcileAt.After(time.Now().UTC()) {
		t.Fatalf("fresh missing receipt did not persist reconciliation backoff: %+v", fresh)
	}
	staleStartedAt := time.Now().UTC().Add(-(service.executionTimeout + sandboxMissingReceiptGrace + time.Second))
	if err := db.Model(&models.MCPExecution{}).
		Where("execution_id = ?", input.ExecutionID).
		Update("started_at", staleStartedAt).Error; err != nil {
		t.Fatal(err)
	}

	terminal, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrExecutionTerminal) {
		t.Fatalf("stale missing receipt should be terminal, execution=%+v err=%v", terminal, err)
	}
	if terminal.Status != models.MCPExecutionStatusTimedOut || !strings.Contains(terminal.ErrorMessage, "SANDBOX_OUTCOME_UNKNOWN") {
		t.Fatalf("stale missing receipt lost outcome classification: %+v", terminal)
	}
	if sandbox.executions != 1 {
		t.Fatalf("missing receipt caused a sandbox replay: %d", sandbox.executions)
	}
}

func TestReceiptIdentityMismatchFailsClosedWithoutReplay(t *testing.T) {
	_, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("response was lost")
	}
	input := ExecuteInput{
		ExecutionID:    "execution-identity-mismatch",
		RunRef:         "workflow:77",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          77,
		ToolCallID:     "call-identity-mismatch",
		ToolName:       "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("expected ambiguous execution, got %v", err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.Status = SandboxExecutionStatusSucceeded
		receipt.RevisionID++
		receipt.Output = map[string]any{"cross_tenant": true}
	})

	execution, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected mismatched receipt to fail closed, execution=%+v err=%v", execution, err)
	}
	if execution.Status != models.MCPExecutionStatusFailed || sandbox.executions != 1 {
		t.Fatalf("identity mismatch was replayed or left active: execution=%+v calls=%d", execution, sandbox.executions)
	}
}

func TestLateReceiptDigestMismatchFailsClosed(t *testing.T) {
	_, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("response was lost")
	}
	input := ExecuteInput{
		ExecutionID: "execution-digest-mismatch", RunRef: "run:78", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 78, ToolCallID: "call-digest-mismatch", ToolName: "mcp.1.search",
		Arguments: map[string]any{"query": "approved input"},
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("expected ambiguous execution, got %v", err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.Status = SandboxExecutionStatusSucceeded
		receipt.RequestDigest = strings.Repeat("f", 64)
		receipt.Output = map[string]any{"wrong": true}
	})

	execution, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrForbidden) || execution.Status != models.MCPExecutionStatusFailed {
		t.Fatalf("digest mismatch did not fail closed: execution=%+v err=%v", execution, err)
	}
	if sandbox.executions != 1 {
		t.Fatalf("digest mismatch caused a replay: %d", sandbox.executions)
	}
}

func TestTerminalPersistenceFailureIsRecoveredFromReceipt(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	injected := errors.New("injected terminal persistence failure")
	failNextTerminalUpdate := true
	callbackName := "test:fail_mcp_terminal_update"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || tx.Statement.Table != "mcp_executions" || !failNextTerminalUpdate {
			return
		}
		if updates["status"] == models.MCPExecutionStatusSucceeded {
			failNextTerminalUpdate = false
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	input := ExecuteInput{
		ExecutionID:    "execution-persist-failure",
		RunRef:         "agent:303",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          303,
		ToolCallID:     "call-persist-failure",
		ToolName:       "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, injected) {
		t.Fatalf("terminal persistence error was swallowed: %v", err)
	}
	if err := db.Callback().Update().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	var active models.MCPExecution
	if err := db.Where("execution_id = ?", input.ExecutionID).Take(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("failed terminal transaction did not roll back: %+v", active)
	}

	recovered, err := service.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("receipt recovery after persistence failure: %v", err)
	}
	if recovered.Status != models.MCPExecutionStatusSucceeded || sandbox.executions != 1 {
		t.Fatalf("persistence recovery replayed side effect: execution=%+v calls=%d", recovered, sandbox.executions)
	}
}

func TestTerminalOutboxFailureRollsBackExecutionTransition(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	injected := errors.New("injected terminal outbox failure")
	failTerminalOutbox := true
	callbackName := "test:fail_mcp_terminal_outbox"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if failTerminalOutbox && tx.Statement.Table == "event_outbox" {
			failTerminalOutbox = false
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	input := ExecuteInput{
		ExecutionID: "execution-outbox-failure", RunRef: "run:304", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 304, ToolCallID: "call-outbox-failure", ToolName: "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, injected) {
		t.Fatalf("terminal outbox error was swallowed: %v", err)
	}
	if err := db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	var active models.MCPExecution
	if err := db.Where("execution_id = ?", input.ExecutionID).Take(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("terminal execution committed without its outbox: %+v", active)
	}
	var eventsBeforeRetry int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ?", EventMCPExecutionTerminal).Count(&eventsBeforeRetry).Error; err != nil {
		t.Fatal(err)
	}
	if eventsBeforeRetry != 0 {
		t.Fatalf("failed terminal transaction leaked %d outbox rows", eventsBeforeRetry)
	}

	recovered, err := service.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("recover after terminal outbox rollback: %v", err)
	}
	if recovered.Status != models.MCPExecutionStatusSucceeded || sandbox.executions != 1 {
		t.Fatalf("outbox recovery replayed side effect: execution=%+v calls=%d", recovered, sandbox.executions)
	}
}

func TestCanceledRequestStillPersistsTerminalExecutionAndOutbox(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	ctx, cancel := context.WithCancel(context.Background())
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		cancel()
		return ExecutionResult{Output: map[string]any{"ok": true}}, nil
	}
	agentRunID := uint64(404)
	input := ExecuteInput{
		ExecutionID:    "execution-canceled-request",
		RunRef:         "agent:404",
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunID:          agentRunID,
		AgentRunID:     &agentRunID,
		ToolCallID:     "call-canceled-request",
		ToolName:       "mcp.1.search",
	}
	execution, err := service.Execute(ctx, input)
	if err != nil {
		t.Fatalf("detached terminal persistence failed: %v", err)
	}
	if execution.Status != models.MCPExecutionStatusSucceeded {
		t.Fatalf("execution did not reach terminal state: %+v", execution)
	}
	var outbox models.EventOutbox
	if err := db.Where("idempotency_key = ?", EventMCPExecutionTerminal+":"+input.ExecutionID).Take(&outbox).Error; err != nil {
		t.Fatalf("terminal outbox was not persisted: %v", err)
	}
	var payload MCPExecutionTerminalEvent
	if err := json.Unmarshal([]byte(outbox.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ExecutionID != input.ExecutionID || payload.AgentRunID == nil || *payload.AgentRunID != agentRunID || payload.WorkflowRunID != nil {
		t.Fatalf("unexpected terminal event payload: %+v", payload)
	}
}

func TestReconcilePendingExecutionsIsolatesReceiptFailures(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("response was lost")
	}
	inputs := []ExecuteInput{
		{
			ExecutionID: "execution-batch-invalid", RunRef: "run:501", OrganizationID: 1, UserID: 7,
			ConversationID: 42, RunID: 501, ToolCallID: "call-batch-invalid", ToolName: "mcp.1.search",
		},
		{
			ExecutionID: "execution-batch-valid", RunRef: "run:502", OrganizationID: 1, UserID: 7,
			ConversationID: 42, RunID: 502, ToolCallID: "call-batch-valid", ToolName: "mcp.1.search",
		},
	}
	for _, input := range inputs {
		if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
			t.Fatalf("seed active execution %q: %v", input.ExecutionID, err)
		}
		sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
			receipt.Status = SandboxExecutionStatusSucceeded
			receipt.Output = map[string]any{"execution_id": input.ExecutionID}
		})
	}
	due := time.Now().UTC().Add(-time.Second)
	if err := db.Model(&models.MCPExecution{}).
		Where("execution_id IN ?", []string{inputs[0].ExecutionID, inputs[1].ExecutionID}).
		Update("next_reconcile_at", due).Error; err != nil {
		t.Fatal(err)
	}
	sandbox.updateReceipt(inputs[0].ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.ToolID++
	})

	count, err := service.ReconcilePendingExecutions(context.Background(), 10)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected aggregate identity error, count=%d err=%v", count, err)
	}
	if count != 2 {
		t.Fatalf("expected both rows to terminalize, got %d", count)
	}
	var statuses []string
	if err := db.Model(&models.MCPExecution{}).Order("execution_id ASC").Pluck("status", &statuses).Error; err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 || statuses[0] != models.MCPExecutionStatusFailed || statuses[1] != models.MCPExecutionStatusSucceeded {
		t.Fatalf("unexpected reconciled statuses: %#v", statuses)
	}
	if sandbox.executions != 2 {
		t.Fatalf("batch reconciliation replayed sandbox calls: %d", sandbox.executions)
	}
}

func TestTransientReceiptLookupPersistsReconciliationBackoff(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("sandbox response was lost")
	}
	sandbox.lookup = func(context.Context, string) (SandboxExecutionReceipt, error) {
		return SandboxExecutionReceipt{}, errors.New("sandbox control plane is temporarily unavailable")
	}
	input := ExecuteInput{
		ExecutionID: "execution-transient-lookup", RunRef: "run:601", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 601, ToolCallID: "call-transient-lookup", ToolName: "mcp.1.search",
	}

	execution, err := service.Execute(context.Background(), input)
	if !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("transient lookup should remain in progress: execution=%+v err=%v", execution, err)
	}
	var stored models.MCPExecution
	if err := db.Where("execution_id = ?", input.ExecutionID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.MCPExecutionStatusRunning || stored.ReconcileAttempts != 1 || stored.NextReconcileAt == nil || !stored.NextReconcileAt.After(time.Now().UTC()) {
		t.Fatalf("transient lookup did not move reconciliation forward: %+v", stored)
	}
}

func TestReconcileMissingIdentityFailsClosedWithTerminalOutbox(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	now := time.Now().UTC()
	execution := models.MCPExecution{
		ExecutionID:          "execution-missing-identity",
		RunRef:               "run:701",
		OrganizationID:       1,
		UserID:               7,
		InstallationID:       999_001,
		RevisionID:           999_002,
		ToolID:               999_003,
		ToolCallID:           "call-missing-identity",
		Status:               models.MCPExecutionStatusRunning,
		InputJSON:            "{}",
		SandboxJobID:         "execution-missing-identity",
		SandboxRequestDigest: strings.Repeat("a", 64),
		NextReconcileAt:      &now,
		ExpiresAt:            now.Add(time.Hour),
	}
	if err := db.Create(&execution).Error; err != nil {
		t.Fatal(err)
	}

	count, err := service.ReconcilePendingExecutions(context.Background(), 10)
	if count != 1 || !errors.Is(err, ErrForbidden) {
		t.Fatalf("deterministic identity error was not terminalized: count=%d err=%v", count, err)
	}
	var stored models.MCPExecution
	if err := db.Take(&stored, execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.MCPExecutionStatusFailed || stored.NextReconcileAt != nil {
		t.Fatalf("identity error remained active: %+v", stored)
	}
	var terminalEvents int64
	if err := db.Model(&models.EventOutbox{}).
		Where("idempotency_key = ?", EventMCPExecutionTerminal+":"+execution.ExecutionID).
		Count(&terminalEvents).Error; err != nil {
		t.Fatal(err)
	}
	if terminalEvents != 1 || sandbox.lookups != 0 {
		t.Fatalf("identity failure outbox/lookups=(%d,%d), want (1,0)", terminalEvents, sandbox.lookups)
	}
}

func TestReconcilePendingExecutionsSkipsMoreThanOneBatchOfDeferredRows(t *testing.T) {
	db, service, sandbox := setupActiveTool(t)
	var tool models.MCPTool
	if err := db.Where("namespaced_name = ?", "mcp.1.search").Take(&tool).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	blockers := make([]models.MCPExecution, 0, 125)
	for index := 0; index < 125; index++ {
		blockers = append(blockers, models.MCPExecution{
			ExecutionID:          fmt.Sprintf("execution-deferred-%03d", index),
			RunRef:               fmt.Sprintf("run:%d", 800+index),
			OrganizationID:       1,
			UserID:               7,
			InstallationID:       tool.InstallationID,
			RevisionID:           tool.RevisionID,
			ToolID:               tool.ID,
			ToolCallID:           fmt.Sprintf("call-deferred-%03d", index),
			Status:               models.MCPExecutionStatusRunning,
			InputJSON:            "{}",
			SandboxJobID:         fmt.Sprintf("execution-deferred-%03d", index),
			SandboxRequestDigest: strings.Repeat("b", 64),
			NextReconcileAt:      &future,
			ExpiresAt:            future,
		})
	}
	if err := db.Create(&blockers).Error; err != nil {
		t.Fatal(err)
	}

	sandbox.execute = func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{}, errors.New("response was lost after receipt creation")
	}
	input := ExecuteInput{
		ExecutionID: "execution-due-after-deferred", RunRef: "run:999", OrganizationID: 1, UserID: 7,
		ConversationID: 42, RunID: 999, ToolCallID: "call-due-after-deferred", ToolName: "mcp.1.search",
	}
	if _, err := service.Execute(context.Background(), input); !errors.Is(err, ErrExecutionInProgress) {
		t.Fatalf("seed due execution: %v", err)
	}
	sandbox.updateReceipt(input.ExecutionID, func(receipt *SandboxExecutionReceipt) {
		receipt.Status = SandboxExecutionStatusSucceeded
		receipt.Output = map[string]any{"reconciled": true}
	})
	past := now.Add(-time.Second)
	if err := db.Model(&models.MCPExecution{}).
		Where("execution_id = ?", input.ExecutionID).
		Update("next_reconcile_at", past).Error; err != nil {
		t.Fatal(err)
	}

	count, err := service.ReconcilePendingExecutions(context.Background(), 100)
	if err != nil || count != 1 {
		t.Fatalf("due receipt behind deferred rows was not reconciled: count=%d err=%v", count, err)
	}
	var completed models.MCPExecution
	if err := db.Where("execution_id = ?", input.ExecutionID).Take(&completed).Error; err != nil {
		t.Fatal(err)
	}
	if completed.Status != models.MCPExecutionStatusSucceeded || completed.NextReconcileAt != nil {
		t.Fatalf("due receipt did not reach terminal state: %+v", completed)
	}
	var deferredAttempts int64
	if err := db.Model(&models.MCPExecution{}).
		Where("execution_id LIKE ? AND reconcile_attempts <> 0", "execution-deferred-%").
		Count(&deferredAttempts).Error; err != nil {
		t.Fatal(err)
	}
	if deferredAttempts != 0 {
		t.Fatalf("not-due rows were reconciled unexpectedly: %d", deferredAttempts)
	}
}
