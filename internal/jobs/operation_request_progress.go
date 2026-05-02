package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	runtimebackend "github.com/dominikpalatynski/toolshed/internal/runtime"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type workspaceManager interface {
	WorkspaceExists(locator string) (bool, error)
	CreateStaging(locator string) (string, error)
	ExtractTarball(stagingPath string, body io.Reader) error
	PromoteStaging(stagingPath, locator string) error
	CleanupStaging(stagingPath string) error
	CleanupWorkspace(locator string) error
	OpenWorkspace(locator string) (runtimebackend.Workspace, error)
}

func (w *OperationRequestWorker) finalizeOperationRequestFailure(
	ctx context.Context,
	operationRequestID int64,
	cause error,
) error {
	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		operationRequest, err := q.GetOperationRequestByID(ctx, operationRequestID)
		if err != nil {
			return fmt.Errorf("reload operation request: %w", err)
		}

		message := cause.Error()
		if _, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateFailed,
		}, operationRequest.RuntimeEnvironmentID, map[string]any{
			"runtime_environment_id": operationRequest.RuntimeEnvironmentID,
			"error":                  message,
		}); err != nil {
			return err
		}

		if _, err := q.MarkOperationRequestFailed(ctx, sqlc.MarkOperationRequestFailedParams{
			ID:                   operationRequest.ID,
			RuntimeEnvironmentID: operationRequest.RuntimeEnvironmentID,
			Outcome:              strPtr(operations.OutcomeOperationFailed),
			LastError:            &message,
		}); err != nil {
			return fmt.Errorf("mark operation request failed: %w", err)
		}

		if operationRequest.OperationType == operations.TypePreviewStart && operationRequest.RuntimeEnvironmentID != nil {
			runtimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, *operationRequest.RuntimeEnvironmentID)
			if err != nil {
				return fmt.Errorf("reload runtime environment for failure handling: %w", err)
			}
			if runtimeEnvironment.Status == operations.RuntimeStatusPreparing {
				if _, err := q.UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
					ID:     runtimeEnvironment.ID,
					Status: operations.RuntimeStatusFailed,
				}); err != nil {
					return fmt.Errorf("mark runtime environment failed: %w", err)
				}
			}
		}

		return nil
	})
}

func (w *OperationRequestWorker) completeOperationRequestTx(
	ctx context.Context,
	q *sqlc.Queries,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironmentID *int64,
	outcome string,
	step operations.StepStatus,
	details any,
) error {
	if _, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, step, runtimeEnvironmentID, details); err != nil {
		return err
	}
	if _, err := q.MarkOperationRequestHandled(ctx, sqlc.MarkOperationRequestHandledParams{
		ID:                   operationRequest.ID,
		RuntimeEnvironmentID: runtimeEnvironmentID,
		Outcome:              &outcome,
	}); err != nil {
		return fmt.Errorf("mark operation request handled: %w", err)
	}
	return nil
}

func (w *OperationRequestWorker) advanceOperationRequestStepTx(
	ctx context.Context,
	q *sqlc.Queries,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironmentID *int64,
	completedStepDetails any,
	nextStep operations.StepStatus,
	nextStepDetails any,
) (sqlc.OperationRequests, error) {
	updated, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
		Name:  operationRequest.CurrentStep,
		State: operations.StepStateCompleted,
	}, runtimeEnvironmentID, completedStepDetails)
	if err != nil {
		return sqlc.OperationRequests{}, err
	}

	return w.transitionOperationRequestStepTx(ctx, q, updated, nextStep, runtimeEnvironmentID, nextStepDetails)
}

func (w *OperationRequestWorker) transitionOperationRequestStepTx(
	ctx context.Context,
	q *sqlc.Queries,
	operationRequest sqlc.OperationRequests,
	nextStep operations.StepStatus,
	runtimeEnvironmentID *int64,
	details any,
) (sqlc.OperationRequests, error) {
	currentStep := operations.StepStatus{
		Name:  operationRequest.CurrentStep,
		State: operationRequest.CurrentStepState,
	}
	if currentStep != nextStep {
		if err := operations.ValidateStepTransition(operationRequest.OperationType, currentStep, nextStep); err != nil {
			return sqlc.OperationRequests{}, err
		}
	}

	detailsJSON, err := marshalStepDetails(details)
	if err != nil {
		return sqlc.OperationRequests{}, err
	}

	updated, err := q.SetOperationRequestProgress(ctx, sqlc.SetOperationRequestProgressParams{
		ID:                     operationRequest.ID,
		RuntimeEnvironmentID:   runtimeEnvironmentID,
		CurrentStep:            nextStep.Name,
		CurrentStepState:       nextStep.State,
		CurrentStepDetailsJson: detailsJSON,
	})
	if err != nil {
		return sqlc.OperationRequests{}, fmt.Errorf("update operation request progress: %w", err)
	}
	return updated, nil
}

func marshalStepDetails(details any) ([]byte, error) {
	if details == nil {
		return nil, nil
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("marshal step details: %w", err)
	}
	return payload, nil
}

func runtimeEnvironmentStepDetails(
	runtimeEnvironment sqlc.RuntimeEnvironments,
	extra map[string]any,
) map[string]any {
	details := map[string]any{
		"runtime_environment_id": runtimeEnvironment.ID,
		"workspace_locator":      stringPtrValue(runtimeEnvironment.WorkspaceLocator),
	}
	for key, value := range extra {
		details[key] = value
	}
	return details
}

func deploymentRecordFromModel(record sqlc.RuntimeEnvironmentDeployments) runtimebackend.DeploymentRecord {
	return runtimebackend.DeploymentRecord{
		Backend:                        record.Backend,
		FrozenRuntimeSettingsJSON:      append([]byte(nil), record.FrozenRuntimeSettingsJson...),
		FrozenEnvironmentVariablesJSON: append([]byte(nil), record.FrozenEnvironmentVariablesJson...),
		MetadataJSON:                   append([]byte(nil), record.DeploymentMetadataJson...),
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
