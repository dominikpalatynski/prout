package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	runtimebackend "github.com/dominikpalatynski/toolshed/internal/runtime"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

func (w *OperationRequestWorker) handlePreviewCleanupSuperseded(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
) error {
	if operationRequest.TargetRuntimeEnvironmentID == nil {
		return errors.New("preview-cleanup-superseded operation is missing target_runtime_environment_id")
	}

	for {
		currentRequest, err := w.store.Q().GetOperationRequestByID(ctx, operationRequest.ID)
		if err != nil {
			return fmt.Errorf("reload cleanup operation request: %w", err)
		}
		if currentRequest.Status == operations.StatusHandled {
			return nil
		}

		runtimeEnvironment, err := w.store.Q().GetRuntimeEnvironmentByID(ctx, *operationRequest.TargetRuntimeEnvironmentID)
		if err != nil {
			return fmt.Errorf("load cleanup target runtime environment: %w", err)
		}

		switch currentRequest.CurrentStep {
		case operations.StepRuntimeTeardown:
			if err := w.runCleanupRuntimeTeardownStep(ctx, currentRequest, runtimeEnvironment); err != nil {
				return err
			}
		case operations.StepWorkspaceCleanup:
			if err := w.runCleanupWorkspaceStep(ctx, currentRequest, runtimeEnvironment); err != nil {
				return err
			}
		default:
			return runtimebackend.PermanentError(fmt.Errorf(
				"cleanup operation is at unsupported step %q",
				currentRequest.CurrentStep,
			))
		}
	}
}

func (w *OperationRequestWorker) runCleanupRuntimeTeardownStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	currentRequest := operationRequest

	deploymentRecord, found, err := w.loadRuntimeEnvironmentDeploymentRecord(ctx, runtimeEnvironment.ID)
	if err != nil {
		return err
	}
	if !found {
		return w.advanceCleanupToWorkspaceStep(ctx, currentRequest, runtimeEnvironment, false)
	}

	if currentRequest.CurrentStepState == operations.StepStatePending {
		if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
			updated, err := w.transitionOperationRequestStepTx(ctx, q, currentRequest, operations.StepStatus{
				Name:  operations.StepRuntimeTeardown,
				State: operations.StepStateInProgress,
			}, &runtimeEnvironment.ID, runtimeEnvironmentStepDetails(runtimeEnvironment, map[string]any{
				"deployment_backend": deploymentRecord.Backend,
			}))
			if err != nil {
				return err
			}
			currentRequest = updated
			return nil
		}); err != nil {
			return err
		}
	}

	workspace, err := w.openRuntimeWorkspace(runtimeEnvironment)
	if err != nil {
		return err
	}

	if err := w.runtimeBackend.Teardown(ctx, runtimebackend.TeardownRequest{
		Workspace:  workspace,
		Deployment: deploymentRecord,
	}); err != nil {
		return err
	}

	return w.advanceCleanupToWorkspaceStep(ctx, currentRequest, runtimeEnvironment, true)
}

func (w *OperationRequestWorker) advanceCleanupToWorkspaceStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
	runtimeTeardownRan bool,
) error {
	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		extra := map[string]any{
			"runtime_teardown_ran": runtimeTeardownRan,
		}
		_, err := w.advanceOperationRequestStepTx(ctx, q, operationRequest, &runtimeEnvironment.ID, runtimeEnvironmentStepDetails(runtimeEnvironment, extra), operations.StepStatus{
			Name:  operations.StepWorkspaceCleanup,
			State: operations.StepStatePending,
		}, runtimeEnvironmentStepDetails(runtimeEnvironment, extra))
		return err
	})
}

func (w *OperationRequestWorker) runCleanupWorkspaceStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	currentRequest := operationRequest
	if currentRequest.CurrentStepState == operations.StepStatePending {
		if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
			updated, err := w.transitionOperationRequestStepTx(ctx, q, currentRequest, operations.StepStatus{
				Name:  operations.StepWorkspaceCleanup,
				State: operations.StepStateInProgress,
			}, &runtimeEnvironment.ID, runtimeEnvironmentStepDetails(runtimeEnvironment, nil))
			if err != nil {
				return err
			}
			currentRequest = updated
			return nil
		}); err != nil {
			return err
		}
	}

	if err := w.workspaceManager.CleanupWorkspace(stringPtrValue(runtimeEnvironment.WorkspaceLocator)); err != nil {
		return fmt.Errorf("cleanup workspace: %w", err)
	}

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		return w.completeOperationRequestTx(ctx, q, currentRequest, &runtimeEnvironment.ID, operations.OutcomeCleanupCompleted, operations.StepStatus{
			Name:  currentRequest.CurrentStep,
			State: operations.StepStateCompleted,
		}, runtimeEnvironmentStepDetails(runtimeEnvironment, nil))
	})
}

func (w *OperationRequestWorker) insertSupersededCleanupOperationRequestTx(
	ctx context.Context,
	q *sqlc.Queries,
	tx pgx.Tx,
	triggeringOperationRequest sqlc.OperationRequests,
	targetRuntimeEnvironment sqlc.RuntimeEnvironments,
) error {
	if w.jobEnqueuer == nil {
		return errors.New("operation request enqueuer is unavailable")
	}

	initialStep, err := operations.InitialStepForOperation(operations.TypePreviewCleanupSuperseded)
	if err != nil {
		return err
	}
	intentSnapshotJSON, err := operations.BuildCleanupSupersededSnapshot(
		targetRuntimeEnvironment.RepositoryID,
		targetRuntimeEnvironment.PullRequestID,
		targetRuntimeEnvironment.ID,
		targetRuntimeEnvironment.TargetPrHeadCommitSha,
		stringPtrValue(targetRuntimeEnvironment.WorkspaceLocator),
	)
	if err != nil {
		return fmt.Errorf("build cleanup operation snapshot: %w", err)
	}

	operationRequest, err := q.InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		WebhookEventID:                  nil,
		WebhookEventTriggerEvaluationID: nil,
		RepositoryID:                    targetRuntimeEnvironment.RepositoryID,
		RepositoryTriggerID:             nil,
		PullRequestID:                   targetRuntimeEnvironment.PullRequestID,
		RuntimeEnvironmentID:            nil,
		TargetRuntimeEnvironmentID:      &targetRuntimeEnvironment.ID,
		OperationType:                   operations.TypePreviewCleanupSuperseded,
		Source:                          operations.SourceSystem,
		Status:                          operations.StatusQueued,
		TargetPrHeadCommitSha:           targetRuntimeEnvironment.TargetPrHeadCommitSha,
		IntentSnapshotJson:              intentSnapshotJSON,
		CurrentStep:                     initialStep.Name,
		CurrentStepState:                initialStep.State,
		CurrentStepDetailsJson:          nil,
	})
	if err != nil {
		return fmt.Errorf("insert cleanup operation request: %w", err)
	}

	if _, err := w.jobEnqueuer.InsertTx(ctx, tx, OperationRequestArgs{
		OperationRequestID: operationRequest.ID,
	}, nil); err != nil {
		return fmt.Errorf("enqueue cleanup operation request: %w", err)
	}

	_ = triggeringOperationRequest
	return nil
}
