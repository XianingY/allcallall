package mcpplatform

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type fakeSandbox struct {
	mu          sync.Mutex
	validations int
	executions  int
	tools       []DiscoveredTool
	validate    func(context.Context, ValidationRequest) (ValidationResult, error)
	execute     func(context.Context, ExecutionRequest) (ExecutionResult, error)
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
	execute := f.execute
	f.mu.Unlock()
	if execute != nil {
		return execute(ctx, request)
	}
	return ExecutionResult{JobID: "job-1", Output: map[string]any{"ok": true}}, nil
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
	return db, NewService(db, nil).WithSandbox(sandbox), sandbox
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
