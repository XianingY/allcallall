package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"strings"
	"time"
)

func (s *Service) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (ExecutionReceipt, error) {
	if s.receipts == nil {
		return ExecutionReceipt{}, ErrReceiptUnavailable
	}
	request = normalizeExecutionRequest(request)
	runner, err := s.runnerForSource(request.SourceType)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	if err := validateExecutionIdentity(request); err != nil {
		return ExecutionReceipt{}, err
	}
	digest, err := executionRequestDigest(request)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	now := time.Now().UTC()
	candidate := models.SandboxExecutionReceipt{
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
		StartedAt:      now,
		StaleAt:        now.Add(time.Duration(request.TimeoutMS)*time.Millisecond + s.receiptStaleGrace),
		ExpiresAt:      now.Add(s.receiptRetention),
	}
	stored, winner, err := s.receipts.Acquire(ctx, candidate)
	if err != nil {
		return ExecutionReceipt{}, fmt.Errorf("%w: %v", ErrReceiptUnavailable, err)
	}
	if !winner {
		if stored.RequestDigest != digest {
			receipt, conversionErr := executionReceiptFromModel(stored)
			if conversionErr != nil {
				return ExecutionReceipt{}, conversionErr
			}
			return receipt, ErrExecutionConflict
		}
		stored, err = s.refreshStaleReceipt(ctx, stored)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		receipt, err := executionReceiptFromModel(stored)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		if stored.Status == models.SandboxExecutionStatusRunning {
			return receipt, ErrExecutionInProgress
		}
		return receipt, nil
	}

	var result mcpplatform.ExecutionResult
	var executionErr error
	executionCtx, executionCancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		time.Duration(request.TimeoutMS)*time.Millisecond,
	)
	defer executionCancel()
	switch request.SourceType {
	case models.MCPInstallationSourceOCI:
		if err := validateDigestPinned(request.Definition.ImageRef); err != nil {
			executionErr = err
		}
	case models.MCPInstallationSourceHTTPS:
		if err := s.validateHTTPSDestination(executionCtx, request.Definition); err != nil {
			executionErr = err
		}
	default:
		executionErr = fmt.Errorf("unsupported MCP source type")
	}
	if executionErr == nil {
		if preparingRunner, ok := runner.(PreparingRunner); ok {
			var prepared PreparedExecution
			prepared, executionErr = preparingRunner.PrepareExecution(executionCtx, request)
			if executionErr == nil {
				defer func() {
					closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer closeCancel()
					if closeErr := prepared.Close(closeCtx); closeErr != nil {
						log.Warn().Err(closeErr).Str("execution_id", request.ExecutionID).Msg("failed to close prepared sandbox execution")
					}
				}()
				persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), terminalReceiptWriteTimeout)
				stored, executionErr = s.receipts.SetJobID(persistCtx, request.ExecutionID, digest, prepared.JobID())
				persistCancel()
				if executionErr == nil {
					result, executionErr = prepared.Execute(executionCtx)
					result.JobID = prepared.JobID()
				}
			}
		} else {
			result, executionErr = runner.Execute(executionCtx, request)
		}
	}

	status := models.SandboxExecutionStatusSucceeded
	errorCode := ""
	errorMessage := ""
	var outputJSON []byte
	if executionErr != nil {
		status, errorCode = receiptFailure(executionErr)
		errorMessage = sanitizeReceiptError(executionErr, request.SecretWrapToken)
	} else {
		outputJSON, err = json.Marshal(result.Output)
		if err != nil {
			executionErr = fmt.Errorf("encode runner output: %w", err)
		} else if len(outputJSON) > request.OutputLimit {
			executionErr = mcpplatform.ErrOutputTooLarge
		}
		if executionErr != nil {
			status, errorCode = receiptFailure(executionErr)
			errorMessage = sanitizeReceiptError(executionErr, request.SecretWrapToken)
			outputJSON = nil
		}
	}
	completedAt := time.Now().UTC()
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalReceiptWriteTimeout)
	defer cancel()
	stored, err = s.receipts.Complete(
		persistCtx,
		request.ExecutionID,
		digest,
		status,
		result.JobID,
		outputJSON,
		errorCode,
		errorMessage,
		completedAt,
	)
	if err != nil {
		if errors.Is(err, ErrReceiptStateChanged) {
			stored, err = s.receipts.Get(persistCtx, request.ExecutionID)
		}
		if err != nil {
			return ExecutionReceipt{}, fmt.Errorf("%w: persist terminal sandbox receipt: %v", ErrReceiptUnavailable, err)
		}
	}
	return executionReceiptFromModel(stored)
}

func (s *Service) runnerForSource(sourceType string) (Runner, error) {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case models.MCPInstallationSourceOCI:
		if s.ociRunner == nil {
			return nil, fmt.Errorf("%w: isolated OCI runner unavailable", ErrImageRejected)
		}
		return s.ociRunner, nil
	case models.MCPInstallationSourceHTTPS:
		if s.runner == nil {
			return nil, fmt.Errorf("HTTPS runner unavailable")
		}
		return s.runner, nil
	default:
		return nil, fmt.Errorf("unsupported MCP source type")
	}
}

func (s *Service) LookupExecution(ctx context.Context, executionID string) (ExecutionReceipt, error) {
	if s.receipts == nil {
		return ExecutionReceipt{}, ErrReceiptUnavailable
	}
	executionID = strings.TrimSpace(executionID)
	if executionID == "" || len(executionID) > 96 {
		return ExecutionReceipt{}, fmt.Errorf("%w: invalid execution id", ErrInvalidExecution)
	}
	stored, err := s.receipts.Get(ctx, executionID)
	if err != nil {
		if !errors.Is(err, ErrReceiptNotFound) {
			return ExecutionReceipt{}, fmt.Errorf("%w: %v", ErrReceiptUnavailable, err)
		}
		return ExecutionReceipt{}, err
	}
	stored, err = s.refreshStaleReceipt(ctx, stored)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	return executionReceiptFromModel(stored)
}

func (s *Service) refreshStaleReceipt(ctx context.Context, receipt *models.SandboxExecutionReceipt) (*models.SandboxExecutionReceipt, error) {
	if receipt == nil || receipt.Status != models.SandboxExecutionStatusRunning || time.Now().UTC().Before(receipt.StaleAt) {
		return receipt, nil
	}
	return s.receipts.MarkStaleOutcomeUnknown(ctx, receipt.ExecutionID, receipt.RequestDigest, time.Now().UTC())
}
