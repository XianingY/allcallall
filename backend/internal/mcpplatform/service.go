package mcpplatform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

type Service struct {
	db                *gorm.DB
	metrics           *metrics.CounterStore
	sandbox           SandboxClient
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

func (s *Service) CreateInstallation(ctx context.Context, organizationID, userID uint64, input CreateInstallationInput) (*models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	input.Scope = normalizeScope(input.Scope)
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" || len(input.DisplayName) > 160 {
		return nil, fmt.Errorf("%w: display_name is required", ErrInvalidInput)
	}
	if input.SourceType != models.MCPInstallationSourceOCI && input.SourceType != models.MCPInstallationSourceHTTPS {
		return nil, fmt.Errorf("%w: source_type must be oci or https", ErrInvalidInput)
	}
	input.InstallationDefinition = normalizeDefinition(input.SourceType, input.InstallationDefinition)
	if err := validateDefinition(input.SourceType, input.InstallationDefinition); err != nil {
		return nil, err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	if input.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, ErrForbidden
	}
	var installation models.MCPInstallation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organization models.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", organizationID).Take(&organization).Error; err != nil {
			return ErrForbidden
		}
		query := tx.Model(&models.MCPInstallation{}).
			Where("organization_id = ? AND scope = ? AND deleted_at IS NULL", organizationID, input.Scope)
		limit := s.organizationLimit
		if input.Scope == models.MCPInstallationScopePersonal {
			query = query.Where("owner_user_id = ?", userID)
			limit = s.personalLimit
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrQuotaExceeded
		}
		installation = models.MCPInstallation{
			OrganizationID: organizationID,
			OwnerUserID:    userID,
			Scope:          input.Scope,
			DisplayName:    input.DisplayName,
			SourceType:     input.SourceType,
			Status:         models.MCPInstallationStatusDraft,
		}
		if err := tx.Create(&installation).Error; err != nil {
			return err
		}
		revision, err := revisionFromDefinition(installation.ID, 1, userID, input.InstallationDefinition)
		if err != nil {
			return err
		}
		return tx.Create(&revision).Error
	})
	if err != nil {
		return nil, err
	}
	return &installation, nil
}

func (s *Service) ListInstallations(ctx context.Context, organizationID, userID uint64) ([]models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var items []models.MCPInstallation
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationScopeOrganization, models.MCPInstallationScopePersonal, userID).
		Order("created_at DESC, id DESC").Find(&items).Error
	return items, err
}

func (s *Service) GetInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, *models.MCPInstallationRevision, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, nil, err
	}
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, false)
	if err != nil {
		return nil, nil, err
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, nil, err
	}
	return installation, revision, nil
}

func (s *Service) UpdateInstallation(ctx context.Context, organizationID, userID, installationID uint64, input UpdateInstallationInput) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if input.DisplayName == nil && input.Definition == nil {
		return nil, fmt.Errorf("%w: no changes provided", ErrInvalidInput)
	}
	if input.DisplayName != nil {
		name := strings.TrimSpace(*input.DisplayName)
		if name == "" || len(name) > 160 {
			return nil, fmt.Errorf("%w: invalid display_name", ErrInvalidInput)
		}
		installation.DisplayName = name
	}
	if input.Definition != nil {
		normalized := normalizeDefinition(installation.SourceType, *input.Definition)
		input.Definition = &normalized
		if err := validateDefinition(installation.SourceType, *input.Definition); err != nil {
			return nil, err
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
			Update("display_name", installation.DisplayName).Error; err != nil {
			return err
		}
		if input.Definition == nil {
			return nil
		}
		var latest int
		if err := tx.Model(&models.MCPInstallationRevision{}).Where("installation_id = ?", installation.ID).
			Select("COALESCE(MAX(revision), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		revision, err := revisionFromDefinition(installation.ID, latest+1, userID, *input.Definition)
		if err != nil {
			return err
		}
		return tx.Create(&revision).Error
	})
	return installation, err
}

func (s *Service) DeleteInstallation(ctx context.Context, organizationID, userID, installationID uint64) error {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return err
	}
	if installation.VaultPath != "" {
		if err := s.secrets.Delete(ctx, installation.VaultPath); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
		"status":     models.MCPInstallationStatusDisabled,
		"deleted_at": now,
		"updated_at": now,
	}).Error
}

func (s *Service) PutSecrets(ctx context.Context, organizationID, userID, installationID uint64, values map[string]string) error {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return err
	}
	clean := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: secret keys and values must be non-empty", ErrInvalidInput)
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return fmt.Errorf("%w: secrets are required", ErrInvalidInput)
	}
	path := fmt.Sprintf("secret/data/allcallall/organizations/%d/mcp/%d", organizationID, installation.ID)
	if err := s.secrets.Put(ctx, path, clean); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Update("vault_path", path).Error
}

func (s *Service) ValidateInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if s.sandbox == nil {
		return nil, ErrSandboxUnavailable
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, err
	}
	definition, err := definitionFromRevision(*revision)
	if err != nil {
		return nil, err
	}
	secretWrapToken, err := s.wrapSecrets(ctx, installation.VaultPath)
	if err != nil {
		return nil, err
	}
	previousStatus := installation.Status
	if err := s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
		Updates(map[string]any{"status": models.MCPInstallationStatusValidating, "last_error": ""}).Error; err != nil {
		return nil, err
	}
	started := time.Now()
	result, validateErr := s.sandbox.Validate(ctx, ValidationRequest{
		InstallationID:  installation.ID,
		RevisionID:      revision.ID,
		SourceType:      installation.SourceType,
		Definition:      definition,
		SecretWrapToken: secretWrapToken,
	})
	if s.metrics != nil {
		s.metrics.Inc("mcp_validation_count")
		s.metrics.Add("mcp_validation_ms_sum", time.Since(started).Milliseconds())
	}
	if validateErr != nil {
		status := models.MCPInstallationStatusFailed
		if installation.ActiveRevisionID != nil && previousStatus == models.MCPInstallationStatusActive {
			status = models.MCPInstallationStatusActive
		}
		_ = s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
			Updates(map[string]any{"status": status, "last_error": sanitizeError(validateErr)}).Error
		return nil, validateErr
	}
	scanStatus := strings.ToLower(strings.TrimSpace(result.ScanStatus))
	if scanStatus != "passed" {
		installationStatus := models.MCPInstallationStatusFailed
		if scanStatus == "critical" || scanStatus == "quarantined" {
			installationStatus = models.MCPInstallationStatusQuarantined
		}
		if err := s.storeValidationResult(ctx, installation, revision, result, installationStatus); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: sandbox validation did not return a passed scan", ErrInvalidState)
	}
	if len(result.Tools) == 0 {
		result.Tools = []DiscoveredTool{}
	}
	nextStatus := models.MCPInstallationStatusDisabled
	if installation.ActiveRevisionID != nil && previousStatus == models.MCPInstallationStatusActive {
		nextStatus = models.MCPInstallationStatusActive
	}
	if err := s.storeValidationResult(ctx, installation, revision, result, nextStatus); err != nil {
		return nil, err
	}
	installation.Status = nextStatus
	return installation, nil
}

func (s *Service) ActivateInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, err
	}
	if revision.ScanStatus != "passed" {
		return nil, fmt.Errorf("%w: latest revision has not passed validation", ErrInvalidState)
	}
	var toolCount int64
	if err := s.db.WithContext(ctx).Model(&models.MCPTool{}).Where("revision_id = ? AND status = 'active'", revision.ID).Count(&toolCount).Error; err != nil {
		return nil, err
	}
	if toolCount == 0 {
		return nil, fmt.Errorf("%w: validated revision has no active tools", ErrInvalidState)
	}
	if err := s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
		"active_revision_id": revision.ID,
		"status":             models.MCPInstallationStatusActive,
		"last_error":         "",
	}).Error; err != nil {
		return nil, err
	}
	installation.ActiveRevisionID = &revision.ID
	installation.Status = models.MCPInstallationStatusActive
	return installation, nil
}

func (s *Service) PublishInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, role, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if !isAdmin(role) || installation.Status != models.MCPInstallationStatusActive || installation.ActiveRevisionID == nil {
		return nil, ErrForbidden
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
			"scope":        models.MCPInstallationScopeOrganization,
			"published_by": userID,
			"published_at": now,
		}).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"revision_id": *installation.ActiveRevisionID})
		return tx.Create(&models.OrganizationAuditEvent{
			OrganizationID: organizationID,
			ActorUserID:    userID,
			Action:         "agent.mcp.installation.published",
			TargetType:     "mcp_installation",
			TargetID:       fmt.Sprintf("%d", installation.ID),
			MetadataJSON:   string(metadata),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	installation.Scope = models.MCPInstallationScopeOrganization
	installation.PublishedBy = &userID
	installation.PublishedAt = &now
	return installation, nil
}

func (s *Service) ListTools(ctx context.Context, organizationID, userID, installationID uint64) ([]models.MCPTool, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, false)
	if err != nil {
		return nil, err
	}
	var tools []models.MCPTool
	query := s.db.WithContext(ctx).Where("installation_id = ?", installation.ID)
	if installation.ActiveRevisionID != nil {
		query = query.Where("revision_id = ?", *installation.ActiveRevisionID)
	} else {
		revision, err := s.latestRevision(ctx, installation.ID)
		if err != nil {
			return nil, err
		}
		query = query.Where("revision_id = ?", revision.ID)
	}
	err = query.Order("original_name ASC").Find(&tools).Error
	return tools, err
}

func (s *Service) Catalog(ctx context.Context, organizationID, userID uint64) ([]models.MCPTool, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var installations []models.MCPInstallation
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND status = ? AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationStatusActive, models.MCPInstallationScopeOrganization,
			models.MCPInstallationScopePersonal, userID).
		Find(&installations).Error; err != nil {
		return nil, err
	}
	revisionIDs := make([]uint64, 0, len(installations))
	for _, installation := range installations {
		if installation.ActiveRevisionID != nil {
			revisionIDs = append(revisionIDs, *installation.ActiveRevisionID)
		}
	}
	if len(revisionIDs) == 0 {
		return []models.MCPTool{}, nil
	}
	var tools []models.MCPTool
	err := s.db.WithContext(ctx).Where("revision_id IN ? AND status = 'active'", revisionIDs).
		Order("namespaced_name ASC").Find(&tools).Error
	return tools, err
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (*models.MCPExecution, error) {
	return s.execute(ctx, input, false)
}

// ExecuteApproved executes a write or unknown-risk tool only after Go-owned approval.
func (s *Service) ExecuteApproved(ctx context.Context, input ExecuteInput) (*models.MCPExecution, error) {
	return s.execute(ctx, input, true)
}

func (s *Service) execute(ctx context.Context, input ExecuteInput, approvalGranted bool) (*models.MCPExecution, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if input.OrganizationID == 0 || input.UserID == 0 || input.RunID == 0 || strings.TrimSpace(input.ToolCallID) == "" {
		return nil, fmt.Errorf("%w: organization, user, run and tool_call_id are required", ErrInvalidInput)
	}
	if input.ExecutionID == "" {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", input.RunID, input.ToolCallID)))
		input.ExecutionID = fmt.Sprintf("mcp:%x", digest[:16])
	}
	input.RunRef = strings.TrimSpace(input.RunRef)
	if input.RunRef == "" {
		input.RunRef = fmt.Sprintf("run:%d", input.RunID)
	}
	tool, installation, err := s.ResolveAuthorizedTool(ctx, input.OrganizationID, input.UserID, input.ToolName)
	if err != nil {
		return nil, err
	}
	if err := validateMCPArguments(tool.InputSchemaJSON, input.Arguments); err != nil {
		return nil, err
	}
	if tool.Risk != models.MCPToolRiskRead && !approvalGranted {
		return nil, ErrApprovalRequired
	}
	inputJSON, err := json.Marshal(input.Arguments)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid arguments", ErrInvalidInput)
	}
	now := time.Now().UTC()
	execution := models.MCPExecution{
		ExecutionID:    input.ExecutionID,
		RunRef:         input.RunRef,
		OrganizationID: input.OrganizationID,
		UserID:         input.UserID,
		AgentRunID:     input.AgentRunID,
		WorkflowRunID:  input.WorkflowRunID,
		InstallationID: installation.ID,
		RevisionID:     tool.RevisionID,
		ToolID:         tool.ID,
		ToolCallID:     input.ToolCallID,
		Status:         models.MCPExecutionStatusQueued,
		InputJSON:      string(inputJSON),
		ExpiresAt:      now.Add(30 * 24 * time.Hour),
	}
	if err := s.db.WithContext(ctx).Create(&execution).Error; err != nil {
		var existing models.MCPExecution
		if lookupErr := s.db.WithContext(ctx).
			Where("execution_id = ? OR (run_ref = ? AND tool_call_id = ?)", input.ExecutionID, input.RunRef, input.ToolCallID).
			Take(&existing).Error; lookupErr == nil {
			if existing.OrganizationID != input.OrganizationID || existing.UserID != input.UserID ||
				existing.ToolID != tool.ID || existing.RevisionID != tool.RevisionID ||
				existing.ExecutionID != input.ExecutionID || existing.RunRef != input.RunRef ||
				existing.ToolCallID != input.ToolCallID || existing.InputJSON != string(inputJSON) {
				return nil, ErrForbidden
			}
			return existingExecutionResult(&existing)
		}
		return nil, err
	}
	if s.sandbox == nil {
		s.failExecution(ctx, execution.ID, ErrSandboxUnavailable)
		return &execution, ErrSandboxUnavailable
	}
	started := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&models.MCPExecution{}).Where("id = ?", execution.ID).Updates(map[string]any{
		"status":     models.MCPExecutionStatusRunning,
		"attempts":   1,
		"started_at": started,
	}).Error
	execution.Status = models.MCPExecutionStatusRunning
	execution.Attempts = 1
	execution.StartedAt = &started
	executionContext, cancel := context.WithTimeout(ctx, s.executionTimeout)
	defer cancel()
	secretWrapToken, err := s.wrapSecrets(executionContext, installation.VaultPath)
	if err != nil {
		s.failExecution(ctx, execution.ID, err)
		return &execution, err
	}
	var revision models.MCPInstallationRevision
	if err := s.db.WithContext(executionContext).Where("id = ?", tool.RevisionID).Take(&revision).Error; err != nil {
		s.failExecution(ctx, execution.ID, err)
		return &execution, err
	}
	definition, err := definitionFromRevision(revision)
	if err != nil {
		s.failExecution(ctx, execution.ID, err)
		return &execution, err
	}
	result, executeErr := s.sandbox.Execute(executionContext, ExecutionRequest{
		ExecutionID:     execution.ExecutionID,
		OrganizationID:  input.OrganizationID,
		UserID:          input.UserID,
		ConversationID:  input.ConversationID,
		RunID:           input.RunID,
		InstallationID:  installation.ID,
		RevisionID:      tool.RevisionID,
		SourceType:      installation.SourceType,
		Definition:      definition,
		ToolName:        tool.OriginalName,
		Arguments:       input.Arguments,
		SecretWrapToken: secretWrapToken,
		TimeoutMS:       s.executionTimeout.Milliseconds(),
		OutputLimit:     s.outputLimit,
	})
	if executeErr != nil {
		s.failExecution(ctx, execution.ID, executeErr)
		return &execution, executeErr
	}
	outputJSON, err := json.Marshal(result.Output)
	if err != nil || len(outputJSON) > s.outputLimit {
		if err == nil {
			err = ErrOutputTooLarge
		}
		s.failExecution(ctx, execution.ID, err)
		return &execution, err
	}
	completed := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&models.MCPExecution{}).Where("id = ?", execution.ID).Updates(map[string]any{
		"status":         models.MCPExecutionStatusSucceeded,
		"output_json":    string(outputJSON),
		"sandbox_job_id": result.JobID,
		"completed_at":   completed,
	}).Error; err != nil {
		return &execution, err
	}
	execution.Status = models.MCPExecutionStatusSucceeded
	execution.OutputJSON = string(outputJSON)
	execution.SandboxJobID = result.JobID
	execution.CompletedAt = &completed
	if s.metrics != nil {
		s.metrics.Inc("mcp_execution_count")
		s.metrics.Add("mcp_execution_ms_sum", completed.Sub(started).Milliseconds())
	}
	return &execution, nil
}

func existingExecutionResult(execution *models.MCPExecution) (*models.MCPExecution, error) {
	if execution == nil {
		return nil, ErrInvalidState
	}
	switch execution.Status {
	case models.MCPExecutionStatusSucceeded:
		return execution, nil
	case models.MCPExecutionStatusQueued, models.MCPExecutionStatusStarting, models.MCPExecutionStatusRunning:
		return execution, ErrExecutionInProgress
	case models.MCPExecutionStatusFailed, models.MCPExecutionStatusTimedOut, models.MCPExecutionStatusCanceled:
		return execution, ErrExecutionTerminal
	default:
		return execution, ErrInvalidState
	}
}

func (s *Service) ResolveAuthorizedTool(ctx context.Context, organizationID, userID uint64, toolName string) (*models.MCPTool, *models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, nil, err
	}
	var tool models.MCPTool
	if err := s.db.WithContext(ctx).Where("namespaced_name = ? AND status = 'active'", strings.TrimSpace(toolName)).
		Order("revision_id DESC").Take(&tool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, tool.InstallationID, false)
	if err != nil {
		return nil, nil, err
	}
	if installation.Status != models.MCPInstallationStatusActive || installation.ActiveRevisionID == nil || *installation.ActiveRevisionID != tool.RevisionID {
		return nil, nil, ErrInvalidState
	}
	return &tool, installation, nil
}

func (s *Service) ValidateArguments(ctx context.Context, organizationID, userID uint64, toolName string, arguments map[string]any) (*models.MCPTool, error) {
	tool, _, err := s.ResolveAuthorizedTool(ctx, organizationID, userID, toolName)
	if err != nil {
		return nil, err
	}
	if err := validateMCPArguments(tool.InputSchemaJSON, arguments); err != nil {
		return nil, err
	}
	return tool, nil
}

func (s *Service) GetExecution(ctx context.Context, organizationID, userID uint64, executionID string) (*models.MCPExecution, error) {
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var execution models.MCPExecution
	if err := s.db.WithContext(ctx).Where("execution_id = ? AND organization_id = ? AND user_id = ?", executionID, organizationID, userID).Take(&execution).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &execution, nil
}

func (s *Service) storeValidationResult(ctx context.Context, installation *models.MCPInstallation, revision *models.MCPInstallationRevision, result ValidationResult, status string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reportJSON, _ := json.Marshal(result.ScanReport)
		scanStatus := strings.ToLower(strings.TrimSpace(result.ScanStatus))
		if scanStatus == "" {
			scanStatus = "failed"
		}
		if err := tx.Model(&models.MCPInstallationRevision{}).Where("id = ?", revision.ID).Updates(map[string]any{
			"image_digest":     result.ImageDigest,
			"scan_status":      scanStatus,
			"scan_report_json": string(reportJSON),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("revision_id = ?", revision.ID).Delete(&models.MCPTool{}).Error; err != nil {
			return err
		}
		for _, discovered := range result.Tools {
			name := strings.TrimSpace(discovered.Name)
			if !mcpToolNamePattern.MatchString(name) {
				return fmt.Errorf("%w: invalid MCP tool name", ErrInvalidInput)
			}
			inputSchema, err := json.Marshal(discovered.InputSchema)
			if err != nil {
				return err
			}
			outputSchema, err := json.Marshal(discovered.OutputSchema)
			if err != nil {
				return err
			}
			if len(inputSchema) > 64*1024 || len(outputSchema) > 64*1024 {
				return fmt.Errorf("%w: MCP tool schema is too large", ErrInvalidInput)
			}
			risk := normalizeRisk(discovered.Risk)
			if risk == models.MCPToolRiskRead && !discovered.RiskVerified {
				risk = models.MCPToolRiskUnknown
			}
			schemaVersion := strings.TrimSpace(discovered.SchemaVersion)
			if schemaVersion == "" {
				digest := sha256.Sum256(inputSchema)
				schemaVersion = fmt.Sprintf("sha256:%x", digest[:8])
			}
			tool := models.MCPTool{
				InstallationID:   installation.ID,
				RevisionID:       revision.ID,
				NamespacedName:   fmt.Sprintf("mcp.%d.%s", installation.ID, name),
				OriginalName:     name,
				Description:      strings.TrimSpace(discovered.Description),
				InputSchemaJSON:  string(inputSchema),
				OutputSchemaJSON: string(outputSchema),
				Risk:             risk,
				Status:           "active",
				SchemaVersion:    schemaVersion,
			}
			if err := tx.Create(&tool).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
			"status":     status,
			"last_error": "",
		}).Error
	})
}

func (s *Service) failExecution(ctx context.Context, executionID uint64, executionErr error) {
	now := time.Now().UTC()
	status := models.MCPExecutionStatusFailed
	if errors.Is(executionErr, context.DeadlineExceeded) {
		status = models.MCPExecutionStatusTimedOut
	}
	_ = s.db.WithContext(ctx).Model(&models.MCPExecution{}).Where("id = ?", executionID).Updates(map[string]any{
		"status":        status,
		"error_message": sanitizeError(executionErr),
		"completed_at":  now,
	}).Error
	if s.metrics != nil {
		s.metrics.Inc("mcp_execution_failure_count")
	}
}

func (s *Service) wrapSecrets(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	token, err := s.secrets.Wrap(ctx, path, time.Minute)
	if err != nil {
		if s.metrics != nil {
			s.metrics.Inc("mcp_secret_unwrap_token_failure_count")
		}
		return "", err
	}
	return token, nil
}

func (s *Service) accessInstallation(ctx context.Context, organizationID, userID, installationID uint64, mutate bool) (*models.MCPInstallation, string, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, "", err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, "", err
	}
	var installation models.MCPInstallation
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND deleted_at IS NULL", installationID, organizationID).Take(&installation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if installation.Scope == models.MCPInstallationScopePersonal && installation.OwnerUserID != userID {
		return nil, "", ErrNotFound
	}
	if mutate && installation.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, "", ErrForbidden
	}
	return &installation, role, nil
}

func (s *Service) organizationRole(ctx context.Context, organizationID, userID uint64) (string, error) {
	var member models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, userID).Take(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrForbidden
		}
		return "", err
	}
	return member.Role, nil
}

func (s *Service) latestRevision(ctx context.Context, installationID uint64) (*models.MCPInstallationRevision, error) {
	var revision models.MCPInstallationRevision
	if err := s.db.WithContext(ctx).Where("installation_id = ?", installationID).Order("revision DESC").Take(&revision).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &revision, nil
}

func revisionFromDefinition(installationID uint64, revisionNumber int, userID uint64, definition InstallationDefinition) (models.MCPInstallationRevision, error) {
	commandJSON, err := json.Marshal(definition.Command)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	argsJSON, err := json.Marshal(definition.Args)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	configJSON, err := json.Marshal(definition.Config)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	allowlistJSON, err := json.Marshal(definition.NetworkAllowlist)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	return models.MCPInstallationRevision{
		InstallationID:       installationID,
		Revision:             revisionNumber,
		Transport:            strings.ToLower(strings.TrimSpace(definition.Transport)),
		ImageRef:             strings.TrimSpace(definition.ImageRef),
		EndpointURL:          strings.TrimSpace(definition.EndpointURL),
		CommandJSON:          string(commandJSON),
		ArgsJSON:             string(argsJSON),
		ConfigJSON:           string(configJSON),
		NetworkAllowlistJSON: string(allowlistJSON),
		ScanStatus:           "pending",
		ScanReportJSON:       "{}",
		CreatedBy:            userID,
	}, nil
}

func definitionFromRevision(revision models.MCPInstallationRevision) (InstallationDefinition, error) {
	definition := InstallationDefinition{
		Transport:   revision.Transport,
		ImageRef:    revision.ImageRef,
		EndpointURL: revision.EndpointURL,
	}
	if err := json.Unmarshal([]byte(revision.CommandJSON), &definition.Command); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.ArgsJSON), &definition.Args); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.ConfigJSON), &definition.Config); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.NetworkAllowlistJSON), &definition.NetworkAllowlist); err != nil {
		return InstallationDefinition{}, err
	}
	return definition, nil
}

func validateDefinition(sourceType string, definition InstallationDefinition) error {
	if err := validateDefinitionConfig(definition.Config); err != nil {
		return err
	}
	transport := strings.ToLower(strings.TrimSpace(definition.Transport))
	switch sourceType {
	case models.MCPInstallationSourceOCI:
		if transport != "stdio" {
			return fmt.Errorf("%w: OCI MCP transport must be stdio", ErrInvalidInput)
		}
		imageRef := strings.TrimSpace(definition.ImageRef)
		parts := strings.Split(imageRef, "@sha256:")
		if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 ||
			strings.Contains(parts[0], "@") || strings.ContainsAny(imageRef, " \t\r\n") {
			return fmt.Errorf("%w: OCI image must be pinned to a sha256 digest", ErrInvalidInput)
		}
	case models.MCPInstallationSourceHTTPS:
		if transport != "http" && transport != "streamable_http" && transport != "sse" {
			return fmt.Errorf("%w: HTTPS MCP transport must be http, streamable_http, or sse", ErrInvalidInput)
		}
		endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
		if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil ||
			endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return fmt.Errorf("%w: endpoint_url must be an HTTPS URL without credentials", ErrInvalidInput)
		}
		if isPrivateHost(endpoint.Hostname()) {
			return fmt.Errorf("%w: private endpoint addresses are not allowed", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported source type", ErrInvalidInput)
	}
	return nil
}

func validateDefinitionConfig(config map[string]any) error {
	for key, value := range config {
		switch key {
		case "read_tools", "write_tools":
			if !isStringList(value) {
				return fmt.Errorf("%w: %s must be a string array", ErrInvalidInput, key)
			}
		case "secret_headers", "secret_env":
			mapping, ok := value.(map[string]any)
			if !ok {
				if typed, typedOK := value.(map[string]string); typedOK {
					mapping = make(map[string]any, len(typed))
					for name, secretKey := range typed {
						mapping[name] = secretKey
					}
				} else {
					return fmt.Errorf("%w: %s must map names to secret keys", ErrInvalidInput, key)
				}
			}
			for name, secretKey := range mapping {
				value, ok := secretKey.(string)
				if strings.TrimSpace(name) == "" || !ok || !mcpToolNamePattern.MatchString(strings.TrimSpace(value)) {
					return fmt.Errorf("%w: %s contains an invalid secret reference", ErrInvalidInput, key)
				}
			}
		default:
			return fmt.Errorf("%w: unsupported installation config key %q", ErrInvalidInput, key)
		}
	}
	return nil
}

func isStringList(value any) bool {
	switch values := value.(type) {
	case []string:
		for _, item := range values {
			if !mcpToolNamePattern.MatchString(strings.TrimSpace(item)) {
				return false
			}
		}
		return true
	case []any:
		for _, item := range values {
			text, ok := item.(string)
			if !ok || !mcpToolNamePattern.MatchString(strings.TrimSpace(text)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validateMCPArguments(schemaJSON string, arguments map[string]any) error {
	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return fmt.Errorf("%w: stored MCP input schema is invalid", ErrInvalidState)
	}
	if schemaType, _ := schema["type"].(string); schemaType != "" && schemaType != "object" {
		return fmt.Errorf("%w: MCP input schema root must be an object", ErrInvalidState)
	}
	for _, name := range schemaStringValues(schema["required"]) {
		if _, ok := arguments[name]; !ok {
			return fmt.Errorf("%w: missing required MCP argument %q", ErrInvalidInput, name)
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for name := range arguments {
			if _, declared := properties[name]; !declared {
				return fmt.Errorf("%w: undeclared MCP argument %q", ErrInvalidInput, name)
			}
		}
	}
	for name, value := range arguments {
		rawProperty, ok := properties[name]
		if !ok {
			continue
		}
		property, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		expected, _ := property["type"].(string)
		if expected != "" && !mcpJSONTypeMatches(expected, value) {
			return fmt.Errorf("%w: MCP argument %q must be %s", ErrInvalidInput, name, expected)
		}
		if enums, ok := property["enum"].([]any); ok && len(enums) > 0 {
			matched := false
			for _, candidate := range enums {
				if fmt.Sprint(candidate) == fmt.Sprint(value) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("%w: MCP argument %q is not an allowed value", ErrInvalidInput, name)
			}
		}
	}
	return nil
}

func schemaStringValues(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func mcpJSONTypeMatches(expected string, value any) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		switch value.(type) {
		case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return true
		default:
			return false
		}
	case "integer":
		switch number := value.(type) {
		case float64:
			return number == float64(int64(number))
		case float32:
			return number == float32(int64(number))
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func normalizeDefinition(sourceType string, definition InstallationDefinition) InstallationDefinition {
	if sourceType != models.MCPInstallationSourceHTTPS || len(definition.NetworkAllowlist) > 0 {
		return definition
	}
	endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
	if err == nil && endpoint.Hostname() != "" {
		definition.NetworkAllowlist = []string{strings.ToLower(endpoint.Hostname())}
	}
	return definition
}

func isPrivateHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func normalizeScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), models.MCPInstallationScopeOrganization) {
		return models.MCPInstallationScopeOrganization
	}
	return models.MCPInstallationScopePersonal
}

func normalizeRisk(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case models.MCPToolRiskRead:
		return models.MCPToolRiskRead
	case models.MCPToolRiskWrite:
		return models.MCPToolRiskWrite
	default:
		return models.MCPToolRiskUnknown
	}
}

func isAdmin(role string) bool {
	return role == models.OrganizationRoleOwner || role == models.OrganizationRoleAdmin
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}
