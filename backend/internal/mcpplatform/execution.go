package mcpplatform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (*models.MCPExecution, error) {
	return s.execute(ctx, input, false)
}

// ExecuteApproved executes a write or unknown-risk tool only after Go-owned approval.
func (s *Service) ExecuteApproved(ctx context.Context, input ExecuteInput) (*models.MCPExecution, error) {
	return s.execute(ctx, input, true)
}

type sandboxExpectedIdentity struct {
	ConversationID uint64
	RunID          uint64
	ToolName       string
	RequestDigest  string
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
	if (input.ExpectedInstallationID != 0 && installation.ID != input.ExpectedInstallationID) ||
		(input.ExpectedRevisionID != 0 && tool.RevisionID != input.ExpectedRevisionID) ||
		(input.ExpectedToolID != 0 && tool.ID != input.ExpectedToolID) {
		return nil, ErrForbidden
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
		ExecutionID:     input.ExecutionID,
		RunRef:          input.RunRef,
		OrganizationID:  input.OrganizationID,
		UserID:          input.UserID,
		AgentRunID:      input.AgentRunID,
		WorkflowRunID:   input.WorkflowRunID,
		InstallationID:  installation.ID,
		RevisionID:      tool.RevisionID,
		ToolID:          tool.ID,
		ToolCallID:      input.ToolCallID,
		Status:          models.MCPExecutionStatusQueued,
		InputJSON:       string(inputJSON),
		NextReconcileAt: &now,
		ExpiresAt:       now.Add(30 * 24 * time.Hour),
	}
	if err := s.db.WithContext(ctx).Create(&execution).Error; err != nil {
		var existing models.MCPExecution
		if lookupErr := s.db.WithContext(ctx).
			Where("execution_id = ? OR (run_ref = ? AND tool_call_id = ?)", input.ExecutionID, input.RunRef, input.ToolCallID).
			Take(&existing).Error; lookupErr == nil {
			if existing.OrganizationID != input.OrganizationID || existing.UserID != input.UserID ||
				existing.InstallationID != installation.ID || existing.ToolID != tool.ID || existing.RevisionID != tool.RevisionID ||
				existing.ExecutionID != input.ExecutionID || existing.RunRef != input.RunRef ||
				existing.ToolCallID != input.ToolCallID || existing.InputJSON != string(inputJSON) ||
				!sameOptionalUint64(existing.AgentRunID, input.AgentRunID) ||
				!sameOptionalUint64(existing.WorkflowRunID, input.WorkflowRunID) {
				return nil, ErrForbidden
			}
			return s.existingExecutionResult(ctx, &existing, sandboxExpectedIdentity{
				ConversationID: input.ConversationID,
				RunID:          input.RunID,
				ToolName:       tool.OriginalName,
				RequestDigest:  existing.SandboxRequestDigest,
			})
		}
		return nil, err
	}
	if s.sandbox == nil {
		return s.returnFailedExecution(ctx, &execution, ErrSandboxUnavailable)
	}
	started := time.Now().UTC()
	startedUpdate := s.db.WithContext(ctx).Model(&models.MCPExecution{}).
		Where("id = ? AND status = ?", execution.ID, models.MCPExecutionStatusQueued).
		Updates(map[string]any{
			"status":            models.MCPExecutionStatusRunning,
			"attempts":          gorm.Expr("attempts + 1"),
			"sandbox_job_id":    execution.ExecutionID,
			"started_at":        started,
			"next_reconcile_at": started,
		})
	if startedUpdate.Error != nil {
		return &execution, startedUpdate.Error
	}
	if startedUpdate.RowsAffected != 1 {
		if err := s.db.WithContext(ctx).Take(&execution, execution.ID).Error; err != nil {
			return nil, err
		}
		return s.existingExecutionResult(ctx, &execution, sandboxExpectedIdentity{
			ConversationID: input.ConversationID,
			RunID:          input.RunID,
			ToolName:       tool.OriginalName,
			RequestDigest:  execution.SandboxRequestDigest,
		})
	}
	execution.Status = models.MCPExecutionStatusRunning
	execution.Attempts = 1
	execution.SandboxJobID = execution.ExecutionID
	execution.StartedAt = &started
	executionContext, cancel := context.WithTimeout(ctx, s.executionTimeout)
	defer cancel()
	secretWrapToken, err := s.wrapSecrets(executionContext, installation.VaultPath)
	if err != nil {
		return s.returnFailedExecution(ctx, &execution, err)
	}
	var revision models.MCPInstallationRevision
	if err := s.db.WithContext(executionContext).Where("id = ?", tool.RevisionID).Take(&revision).Error; err != nil {
		return s.returnFailedExecution(ctx, &execution, err)
	}
	definition, err := definitionFromRevision(revision)
	if err != nil {
		return s.returnFailedExecution(ctx, &execution, err)
	}
	request := ExecutionRequest{
		ExecutionID:     execution.ExecutionID,
		OrganizationID:  input.OrganizationID,
		UserID:          input.UserID,
		ConversationID:  input.ConversationID,
		RunID:           input.RunID,
		RunRef:          input.RunRef,
		ToolCallID:      input.ToolCallID,
		InstallationID:  installation.ID,
		RevisionID:      tool.RevisionID,
		ToolID:          tool.ID,
		SourceType:      installation.SourceType,
		Definition:      definition,
		ToolName:        tool.OriginalName,
		Arguments:       input.Arguments,
		SecretWrapToken: secretWrapToken,
		TimeoutMS:       s.executionTimeout.Milliseconds(),
		OutputLimit:     s.outputLimit,
	}
	requestDigest, err := ExecutionRequestDigest(request)
	if err != nil {
		return s.returnFailedExecution(ctx, &execution, err)
	}
	digestUpdate := s.db.WithContext(executionContext).Model(&models.MCPExecution{}).
		Where("id = ? AND status = ? AND (sandbox_request_digest = '' OR sandbox_request_digest = ?)", execution.ID, models.MCPExecutionStatusRunning, requestDigest).
		Update("sandbox_request_digest", requestDigest)
	if digestUpdate.Error != nil {
		return &execution, digestUpdate.Error
	}
	if digestUpdate.RowsAffected == 0 {
		var stored models.MCPExecution
		if err := s.db.WithContext(executionContext).Where("id = ?", execution.ID).Take(&stored).Error; err != nil {
			return &execution, err
		}
		if stored.Status != models.MCPExecutionStatusRunning || stored.SandboxRequestDigest != requestDigest {
			return s.returnFailedExecution(ctx, &execution, fmt.Errorf("%w: MCP execution request digest changed", ErrForbidden))
		}
	}
	execution.SandboxRequestDigest = requestDigest
	identity := sandboxExpectedIdentity{
		ConversationID: input.ConversationID,
		RunID:          input.RunID,
		ToolName:       tool.OriginalName,
		RequestDigest:  requestDigest,
	}
	result, executeErr := s.sandbox.Execute(executionContext, request)
	if executeErr != nil {
		reconciled, _, reconcileErr := s.reconcileExecution(ctx, &execution, &identity)
		if reconcileErr == nil || errors.Is(reconcileErr, ErrExecutionTerminal) || errors.Is(reconcileErr, ErrForbidden) {
			return reconciled, reconcileErr
		}
		return reconciled, fmt.Errorf("%w: sandbox request outcome is ambiguous: %v", ErrExecutionInProgress, executeErr)
	}
	completed, _, completeErr := s.applySandboxReceipt(ctx, &execution, identity, result)
	return completed, completeErr
}

func (s *Service) existingExecutionResult(ctx context.Context, execution *models.MCPExecution, identity sandboxExpectedIdentity) (*models.MCPExecution, error) {
	if execution == nil {
		return nil, ErrInvalidState
	}
	switch execution.Status {
	case models.MCPExecutionStatusSucceeded:
		return execution, nil
	case models.MCPExecutionStatusQueued, models.MCPExecutionStatusStarting, models.MCPExecutionStatusRunning:
		reconciled, _, err := s.reconcileExecution(ctx, execution, &identity)
		return reconciled, err
	case models.MCPExecutionStatusFailed, models.MCPExecutionStatusTimedOut, models.MCPExecutionStatusCanceled:
		return execution, ErrExecutionTerminal
	default:
		return execution, ErrInvalidState
	}
}

func sameOptionalUint64(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Service) reconcileExecution(ctx context.Context, execution *models.MCPExecution, expected *sandboxExpectedIdentity) (*models.MCPExecution, bool, error) {
	if execution == nil {
		return nil, false, ErrInvalidState
	}
	if s.sandbox == nil {
		scheduled, scheduleErr := s.deferExecutionReconciliation(ctx, execution, time.Now().UTC())
		return scheduled, false, errors.Join(ErrExecutionInProgress, ErrSandboxUnavailable, scheduleErr)
	}
	identity := sandboxExpectedIdentity{}
	if expected != nil {
		identity = *expected
	} else {
		loaded, err := s.loadSandboxExpectedIdentity(ctx, execution)
		if err != nil {
			if isDeterministicReconciliationIdentityError(err) {
				contractErr := fmt.Errorf("%w: sandbox execution identity cannot be recovered: %v", ErrForbidden, err)
				failed, transitioned, persistErr := s.failExecution(ctx, execution, contractErr)
				if persistErr != nil {
					return failed, transitioned, errors.Join(contractErr, persistErr)
				}
				return failed, transitioned, contractErr
			}
			scheduled, scheduleErr := s.deferExecutionReconciliation(ctx, execution, time.Now().UTC())
			return scheduled, false, errors.Join(err, scheduleErr)
		}
		identity = loaded
	}
	receipt, err := s.sandbox.LookupExecution(ctx, execution.ExecutionID)
	if err != nil {
		now := time.Now().UTC()
		if errors.Is(err, ErrSandboxExecutionNotFound) && s.executionReceiptDeadlineExpired(execution, now) {
			outcomeErr := errors.New("SANDBOX_OUTCOME_UNKNOWN: sandbox receipt was not created before the execution recovery deadline; automatic replay is disabled")
			completed, transitioned, persistErr := s.completeExecution(
				ctx,
				execution,
				models.MCPExecutionStatusTimedOut,
				"",
				execution.ExecutionID,
				sanitizeError(outcomeErr),
			)
			if persistErr != nil {
				return completed, transitioned, persistErr
			}
			return completed, transitioned, fmt.Errorf("%w: %v", ErrExecutionTerminal, outcomeErr)
		}
		scheduled, scheduleErr := s.deferExecutionReconciliation(ctx, execution, now)
		return scheduled, false, errors.Join(ErrExecutionInProgress, fmt.Errorf("lookup sandbox execution receipt: %w", err), scheduleErr)
	}
	reconciled, transitioned, reconcileErr := s.applySandboxReceipt(ctx, execution, identity, receipt)
	if !transitioned && errors.Is(reconcileErr, ErrExecutionInProgress) {
		scheduled, scheduleErr := s.deferExecutionReconciliation(ctx, reconciled, time.Now().UTC())
		return scheduled, false, errors.Join(reconcileErr, scheduleErr)
	}
	return reconciled, transitioned, reconcileErr
}

func isDeterministicReconciliationIdentityError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrInvalidState)
}

func (s *Service) deferExecutionReconciliation(ctx context.Context, execution *models.MCPExecution, now time.Time) (*models.MCPExecution, error) {
	if execution == nil || execution.ID == 0 {
		return execution, ErrInvalidState
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	attempt := execution.ReconcileAttempts + 1
	next := now.Add(mcpReconcileDelay(attempt))
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mcpReconcilePersistenceWindow)
	defer cancel()
	updated := s.db.WithContext(persistCtx).Model(&models.MCPExecution{}).
		Where("id = ? AND status IN ?", execution.ID, []string{
			models.MCPExecutionStatusQueued,
			models.MCPExecutionStatusStarting,
			models.MCPExecutionStatusRunning,
		}).
		Updates(map[string]any{
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"next_reconcile_at":  next,
			"updated_at":         now,
		})
	if updated.Error != nil {
		return execution, fmt.Errorf("schedule MCP execution reconciliation: %w", updated.Error)
	}
	if updated.RowsAffected == 0 {
		var stored models.MCPExecution
		if err := s.db.WithContext(persistCtx).Where("id = ?", execution.ID).Take(&stored).Error; err != nil {
			return execution, fmt.Errorf("load MCP execution after reconciliation state change: %w", err)
		}
		return &stored, nil
	}
	execution.ReconcileAttempts = attempt
	execution.NextReconcileAt = &next
	execution.UpdatedAt = now
	return execution, nil
}

func mcpReconcileDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := mcpReconcileBaseDelay
	for current := 1; current < attempt && delay < mcpReconcileMaximumDelay; current++ {
		delay *= 2
	}
	if delay > mcpReconcileMaximumDelay {
		return mcpReconcileMaximumDelay
	}
	return delay
}

func (s *Service) executionReceiptDeadlineExpired(execution *models.MCPExecution, now time.Time) bool {
	if execution == nil {
		return false
	}
	startedAt := execution.CreatedAt
	if execution.StartedAt != nil {
		startedAt = *execution.StartedAt
	}
	timeout := s.executionTimeout
	if timeout <= 0 || timeout > DefaultExecutionTimeout {
		timeout = DefaultExecutionTimeout
	}
	return !startedAt.IsZero() && !now.Before(startedAt.Add(timeout+sandboxMissingReceiptGrace))
}

func (s *Service) applySandboxReceipt(
	ctx context.Context,
	execution *models.MCPExecution,
	expected sandboxExpectedIdentity,
	receipt SandboxExecutionReceipt,
) (*models.MCPExecution, bool, error) {
	if err := validateSandboxReceiptIdentity(execution, expected, receipt); err != nil {
		contractErr := fmt.Errorf("%w: %v", ErrForbidden, err)
		failed, transitioned, persistErr := s.failExecution(ctx, execution, contractErr)
		if persistErr != nil {
			return failed, transitioned, errors.Join(contractErr, persistErr)
		}
		return failed, transitioned, contractErr
	}

	switch receipt.Status {
	case SandboxExecutionStatusQueued, SandboxExecutionStatusStarting, SandboxExecutionStatusRunning:
		return execution, false, ErrExecutionInProgress
	case SandboxExecutionStatusSucceeded:
		outputJSON, err := json.Marshal(receipt.Output)
		if err != nil {
			failed, transitioned, persistErr := s.failExecution(ctx, execution, err)
			if persistErr != nil {
				return failed, transitioned, errors.Join(err, persistErr)
			}
			return failed, transitioned, errors.Join(ErrExecutionTerminal, err)
		}
		if len(outputJSON) > s.outputLimit {
			failed, transitioned, persistErr := s.failExecution(ctx, execution, ErrOutputTooLarge)
			if persistErr != nil {
				return failed, transitioned, errors.Join(ErrOutputTooLarge, persistErr)
			}
			return failed, transitioned, errors.Join(ErrExecutionTerminal, ErrOutputTooLarge)
		}
		return s.completeExecution(ctx, execution, models.MCPExecutionStatusSucceeded, string(outputJSON), receipt.JobID, "")
	case SandboxExecutionStatusFailed:
		completed, transitioned, err := s.completeExecution(
			ctx,
			execution,
			models.MCPExecutionStatusFailed,
			"",
			receipt.JobID,
			receiptFailureMessage(receipt),
		)
		if err != nil {
			return completed, transitioned, err
		}
		return completed, transitioned, sandboxReceiptTerminalError(receipt)
	case SandboxExecutionStatusOutcomeUnknown:
		completed, transitioned, err := s.completeExecution(
			ctx,
			execution,
			models.MCPExecutionStatusTimedOut,
			"",
			receipt.JobID,
			receiptFailureMessage(receipt),
		)
		if err != nil {
			return completed, transitioned, err
		}
		return completed, transitioned, sandboxReceiptTerminalError(receipt)
	case SandboxExecutionStatusTimedOut:
		completed, transitioned, err := s.completeExecution(
			ctx,
			execution,
			models.MCPExecutionStatusTimedOut,
			"",
			receipt.JobID,
			receiptFailureMessage(receipt),
		)
		if err != nil {
			return completed, transitioned, err
		}
		return completed, transitioned, errors.Join(sandboxReceiptTerminalError(receipt), context.DeadlineExceeded)
	case SandboxExecutionStatusCanceled:
		completed, transitioned, err := s.completeExecution(
			ctx,
			execution,
			models.MCPExecutionStatusCanceled,
			"",
			receipt.JobID,
			receiptFailureMessage(receipt),
		)
		if err != nil {
			return completed, transitioned, err
		}
		return completed, transitioned, errors.Join(sandboxReceiptTerminalError(receipt), context.Canceled)
	default:
		contractErr := fmt.Errorf("%w: unsupported sandbox execution status %q", ErrInvalidState, receipt.Status)
		failed, transitioned, persistErr := s.failExecution(ctx, execution, contractErr)
		if persistErr != nil {
			return failed, transitioned, errors.Join(contractErr, persistErr)
		}
		return failed, transitioned, contractErr
	}
}

func validateSandboxReceiptIdentity(execution *models.MCPExecution, expected sandboxExpectedIdentity, receipt SandboxExecutionReceipt) error {
	if execution == nil {
		return errors.New("execution is required")
	}
	if strings.TrimSpace(receipt.RequestDigest) == "" {
		return errors.New("sandbox receipt request digest is required")
	}
	if strings.TrimSpace(expected.RequestDigest) == "" {
		return errors.New("persisted sandbox request digest is required")
	}
	if receipt.RequestDigest != expected.RequestDigest {
		return errors.New("sandbox receipt request digest does not match")
	}
	if receipt.ExecutionID != execution.ExecutionID ||
		receipt.OrganizationID != execution.OrganizationID ||
		receipt.UserID != execution.UserID ||
		receipt.RunRef != execution.RunRef ||
		receipt.ToolCallID != execution.ToolCallID ||
		receipt.InstallationID != execution.InstallationID ||
		receipt.RevisionID != execution.RevisionID ||
		receipt.ToolID != execution.ToolID {
		return errors.New("sandbox receipt execution identity does not match")
	}
	if expected.RunID == 0 || receipt.RunID != expected.RunID {
		return errors.New("sandbox receipt run does not match")
	}
	if expected.ConversationID != 0 && receipt.ConversationID != expected.ConversationID {
		return errors.New("sandbox receipt conversation does not match")
	}
	if expected.ToolName == "" || receipt.ToolName != expected.ToolName {
		return errors.New("sandbox receipt tool does not match")
	}
	return nil
}

func (s *Service) loadSandboxExpectedIdentity(ctx context.Context, execution *models.MCPExecution) (sandboxExpectedIdentity, error) {
	if execution == nil {
		return sandboxExpectedIdentity{}, ErrInvalidState
	}
	var tool models.MCPTool
	if err := s.db.WithContext(ctx).
		Where("id = ? AND installation_id = ? AND revision_id = ?", execution.ToolID, execution.InstallationID, execution.RevisionID).
		Take(&tool).Error; err != nil {
		return sandboxExpectedIdentity{}, fmt.Errorf("load execution tool identity: %w", err)
	}
	identity := sandboxExpectedIdentity{ToolName: tool.OriginalName, RequestDigest: execution.SandboxRequestDigest}
	if execution.AgentRunID != nil && execution.WorkflowRunID != nil {
		return sandboxExpectedIdentity{}, fmt.Errorf("%w: execution has multiple parent runs", ErrInvalidState)
	}
	if execution.AgentRunID != nil {
		var run models.AgentRun
		if err := s.db.WithContext(ctx).Where("id = ?", *execution.AgentRunID).Take(&run).Error; err != nil {
			return sandboxExpectedIdentity{}, fmt.Errorf("load execution agent run identity: %w", err)
		}
		if run.OrganizationID != execution.OrganizationID || run.UserID != execution.UserID || execution.RunRef != fmt.Sprintf("agent:%d", run.ID) {
			return sandboxExpectedIdentity{}, ErrForbidden
		}
		identity.ConversationID = run.ConversationID
		identity.RunID = run.ID
		return identity, nil
	}
	if execution.WorkflowRunID != nil {
		var run models.WorkflowRun
		if err := s.db.WithContext(ctx).Where("id = ?", *execution.WorkflowRunID).Take(&run).Error; err != nil {
			return sandboxExpectedIdentity{}, fmt.Errorf("load execution workflow run identity: %w", err)
		}
		if run.OrganizationID != execution.OrganizationID || run.UserID != execution.UserID || execution.RunRef != fmt.Sprintf("workflow:%d", run.ID) {
			return sandboxExpectedIdentity{}, ErrForbidden
		}
		identity.ConversationID = run.ConversationID
		identity.RunID = run.ID
		return identity, nil
	}
	separator := strings.LastIndexByte(execution.RunRef, ':')
	if separator < 0 || separator == len(execution.RunRef)-1 {
		return sandboxExpectedIdentity{}, fmt.Errorf("%w: execution run reference is invalid", ErrInvalidState)
	}
	runID, err := strconv.ParseUint(execution.RunRef[separator+1:], 10, 64)
	if err != nil || runID == 0 {
		return sandboxExpectedIdentity{}, fmt.Errorf("%w: execution run reference is invalid", ErrInvalidState)
	}
	identity.RunID = runID
	return identity, nil
}

func receiptFailureMessage(receipt SandboxExecutionReceipt) string {
	message := strings.TrimSpace(receipt.ErrorMessage)
	code := strings.TrimSpace(receipt.ErrorCode)
	if code == "" {
		return sanitizeError(errors.New(message))
	}
	if message == "" {
		return sanitizeError(errors.New(code))
	}
	return sanitizeError(fmt.Errorf("%s: %s", code, message))
}

func sandboxReceiptTerminalError(receipt SandboxExecutionReceipt) error {
	message := receiptFailureMessage(receipt)
	if message == "" {
		message = receipt.Status
	}
	return fmt.Errorf("%w: %s", ErrExecutionTerminal, message)
}

func (s *Service) returnFailedExecution(ctx context.Context, execution *models.MCPExecution, cause error) (*models.MCPExecution, error) {
	failed, _, persistErr := s.failExecution(ctx, execution, cause)
	if persistErr != nil {
		return failed, errors.Join(cause, persistErr)
	}
	return failed, cause
}

func (s *Service) failExecution(ctx context.Context, execution *models.MCPExecution, cause error) (*models.MCPExecution, bool, error) {
	status := models.MCPExecutionStatusFailed
	if errors.Is(cause, context.DeadlineExceeded) {
		status = models.MCPExecutionStatusTimedOut
	}
	return s.completeExecution(ctx, execution, status, "", execution.SandboxJobID, sanitizeError(cause))
}

func (s *Service) completeExecution(
	ctx context.Context,
	execution *models.MCPExecution,
	status string,
	outputJSON string,
	sandboxJobID string,
	errorMessage string,
) (*models.MCPExecution, bool, error) {
	if execution == nil || execution.ID == 0 || !isMCPExecutionTerminalStatus(status) {
		return execution, false, ErrInvalidState
	}
	if sandboxJobID == "" {
		sandboxJobID = execution.ExecutionID
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	completedAt := time.Now().UTC()
	transitioned := false
	stored := *execution
	err := s.db.WithContext(persistCtx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&models.MCPExecution{}).
			Where("id = ? AND status IN ?", execution.ID, []string{
				models.MCPExecutionStatusQueued,
				models.MCPExecutionStatusStarting,
				models.MCPExecutionStatusRunning,
			}).
			Updates(map[string]any{
				"status":            status,
				"output_json":       outputJSON,
				"sandbox_job_id":    sandboxJobID,
				"error_message":     errorMessage,
				"completed_at":      completedAt,
				"next_reconcile_at": nil,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			if err := tx.Where("id = ?", execution.ID).Take(&stored).Error; err != nil {
				return err
			}
			if !isMCPExecutionTerminalStatus(stored.Status) {
				return ErrInvalidState
			}
			if stored.Status != status ||
				(stored.SandboxJobID != "" && stored.SandboxJobID != sandboxJobID) ||
				(status == models.MCPExecutionStatusSucceeded && stored.OutputJSON != outputJSON) {
				return fmt.Errorf("%w: MCP execution terminal result changed", ErrInvalidState)
			}
			return nil
		}
		transitioned = true
		stored.Status = status
		stored.OutputJSON = outputJSON
		stored.SandboxJobID = sandboxJobID
		stored.ErrorMessage = errorMessage
		stored.CompletedAt = &completedAt
		stored.NextReconcileAt = nil
		if s.outbox == nil {
			return nil
		}
		_, err := s.outbox.EnqueueTx(persistCtx, tx, events.EnqueueInput{
			AggregateType:  "mcp_execution",
			AggregateID:    execution.ID,
			Event:          EventMCPExecutionTerminal,
			IdempotencyKey: EventMCPExecutionTerminal + ":" + execution.ExecutionID,
			Payload: MCPExecutionTerminalEvent{
				ExecutionID:    execution.ExecutionID,
				MCPExecutionID: execution.ID,
				AgentRunID:     execution.AgentRunID,
				WorkflowRunID:  execution.WorkflowRunID,
				Status:         status,
			},
		})
		if errors.Is(err, events.ErrOutboxEventExists) {
			return nil
		}
		return err
	})
	if err != nil {
		return execution, false, fmt.Errorf("persist MCP execution terminal state: %w", err)
	}
	if transitioned && s.metrics != nil {
		if status == models.MCPExecutionStatusSucceeded {
			s.metrics.Inc("mcp_execution_count")
			if execution.StartedAt != nil {
				s.metrics.Add("mcp_execution_ms_sum", completedAt.Sub(*execution.StartedAt).Milliseconds())
			}
		} else {
			s.metrics.Inc("mcp_execution_failure_count")
		}
	}
	return &stored, transitioned, nil
}

func isMCPExecutionTerminalStatus(status string) bool {
	switch status {
	case models.MCPExecutionStatusSucceeded,
		models.MCPExecutionStatusFailed,
		models.MCPExecutionStatusTimedOut,
		models.MCPExecutionStatusCanceled:
		return true
	default:
		return false
	}
}

// ReconcilePendingExecutions resolves durable Sandbox receipts without
// replaying untrusted tools. Errors are isolated so one corrupt receipt does
// not prevent later executions in the batch from advancing.
func (s *Service) ReconcilePendingExecutions(ctx context.Context, limit int) (int, error) {
	if err := s.checkEnabled(); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var executions []models.MCPExecution
	if err := s.db.WithContext(ctx).
		Where("status IN ?", []string{
			models.MCPExecutionStatusQueued,
			models.MCPExecutionStatusStarting,
			models.MCPExecutionStatusRunning,
		}).
		Where("next_reconcile_at IS NULL OR next_reconcile_at <= ?", time.Now().UTC()).
		Order("next_reconcile_at ASC, id ASC").
		Limit(limit).
		Find(&executions).Error; err != nil {
		return 0, err
	}

	reconciled := 0
	var aggregateErr error
	for index := range executions {
		_, transitioned, err := s.reconcileExecution(ctx, &executions[index], nil)
		if transitioned {
			reconciled++
		}
		switch {
		case err == nil, errors.Is(err, ErrExecutionTerminal):
			continue
		case errors.Is(err, ErrExecutionInProgress) &&
			(err == ErrExecutionInProgress || errors.Is(err, ErrSandboxExecutionNotFound)):
			continue
		default:
			aggregateErr = errors.Join(aggregateErr, fmt.Errorf("reconcile MCP execution %q: %w", executions[index].ExecutionID, err))
		}
	}
	return reconciled, aggregateErr
}
