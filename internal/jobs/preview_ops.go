package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type workspaceManager interface {
	WorkspaceExists(locator string) (bool, error)
	CreateStaging(locator string) (string, error)
	ExtractTarball(stagingPath string, body io.Reader) error
	PromoteStaging(stagingPath, locator string) error
	CleanupStaging(stagingPath string) error
	CleanupWorkspace(locator string) error
}

type previewStartMaterialization struct {
	RuntimeEnvironment sqlc.RuntimeEnvironments
}

func (w *OperationRequestWorker) handlePreviewStart(ctx context.Context, operationRequest sqlc.OperationRequests) error {
	snapshot, err := operations.ParsePreviewStartSnapshot(operationRequest.IntentSnapshotJson)
	if err != nil {
		return err
	}
	if !snapshot.Target.PullRequestSourceRepo.IsComplete() {
		return errors.New("preview-start snapshot is missing pull-request source repository details")
	}

	var materialization *previewStartMaterialization
	if operationRequest.RuntimeEnvironmentID != nil {
		materialization, err = w.resumeOwnedPreviewStart(ctx, operationRequest)
	} else {
		materialization, err = w.startPreviewStart(ctx, operationRequest, snapshot)
	}
	if err != nil || materialization == nil {
		return err
	}

	return w.materializePreviewStart(ctx, operationRequest, snapshot, *materialization)
}

func (w *OperationRequestWorker) resumeOwnedPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
) (*previewStartMaterialization, error) {
	var materialization *previewStartMaterialization

	err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		runtimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, *operationRequest.RuntimeEnvironmentID)
		if err != nil {
			return fmt.Errorf("load runtime environment: %w", err)
		}
		workspaceLocator := stringPtrValue(runtimeEnvironment.WorkspaceLocator)

		workspaceExists, err := w.workspaceManager.WorkspaceExists(workspaceLocator)
		if err != nil {
			return fmt.Errorf("check workspace existence: %w", err)
		}

		switch runtimeEnvironment.Status {
		case operations.RuntimeStatusPrepared:
			if !workspaceExists {
				return errors.New("prepared runtime environment is missing its workspace")
			}
			if err := w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeNewAttemptCreated, operations.StepStatus{
				Name:  operationRequest.CurrentStep,
				State: operations.StepStateCompleted,
			}, map[string]any{
				"runtime_environment_id": runtimeEnvironment.ID,
				"workspace_locator":      workspaceLocator,
			}); err != nil {
				return err
			}
			return nil
		case operations.RuntimeStatusSuperseded:
			if err := w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
				Name:  operationRequest.CurrentStep,
				State: operations.StepStateFailed,
			}, map[string]any{
				"runtime_environment_id": runtimeEnvironment.ID,
				"reason":                 "runtime_environment_superseded",
			}); err != nil {
				return err
			}
			return nil
		case operations.RuntimeStatusPreparing:
			if workspaceExists {
				runtimeEnvironment, err = q.UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
					ID:     runtimeEnvironment.ID,
					Status: operations.RuntimeStatusPrepared,
				})
				if err != nil {
					return fmt.Errorf("mark runtime environment prepared after retry: %w", err)
				}
				if err := w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeNewAttemptCreated, operations.StepStatus{
					Name:  operationRequest.CurrentStep,
					State: operations.StepStateCompleted,
				}, map[string]any{
					"runtime_environment_id": runtimeEnvironment.ID,
					"workspace_locator":      workspaceLocator,
				}); err != nil {
					return err
				}
				return nil
			}

			if _, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
				Name:  operationRequest.CurrentStep,
				State: operations.StepStateInProgress,
			}, &runtimeEnvironment.ID, map[string]any{
				"runtime_environment_id": runtimeEnvironment.ID,
				"workspace_locator":      workspaceLocator,
			}); err != nil {
				return err
			}

			materialization = &previewStartMaterialization{RuntimeEnvironment: runtimeEnvironment}
			return nil
		default:
			return fmt.Errorf("runtime environment %d has unsupported status %q", runtimeEnvironment.ID, runtimeEnvironment.Status)
		}
	})
	if err != nil {
		return nil, err
	}

	return materialization, nil
}

func (w *OperationRequestWorker) startPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	snapshot operations.PreviewStartSnapshot,
) (*previewStartMaterialization, error) {
	var materialization *previewStartMaterialization

	err := w.store.Tx(ctx, func(q *sqlc.Queries, tx pgx.Tx) error {
		runtimeEnvironmentType, err := operations.RuntimeEnvironmentTypeForOperation(operationRequest.OperationType)
		if err != nil {
			return err
		}

		sameTargetEnvironment, err := q.GetLatestRuntimeEnvironmentByTarget(ctx, sqlc.GetLatestRuntimeEnvironmentByTargetParams{
			RepositoryID:          operationRequest.RepositoryID,
			PullRequestID:         operationRequest.PullRequestID,
			Type:                  runtimeEnvironmentType,
			TargetPrHeadCommitSha: operationRequest.TargetPrHeadCommitSha,
		})
		switch {
		case err == nil:
			switch sameTargetEnvironment.Status {
			case operations.RuntimeStatusPreparing:
				return w.completeOperationRequestTx(ctx, q, operationRequest, &sameTargetEnvironment.ID, operations.OutcomeAlreadyPreparing, operations.StepStatus{
					Name:  operationRequest.CurrentStep,
					State: operations.StepStateCompleted,
				}, map[string]any{
					"runtime_environment_id": sameTargetEnvironment.ID,
					"resolution":             operations.OutcomeAlreadyPreparing,
				})
			case operations.RuntimeStatusPrepared:
				workspaceExists, existsErr := w.workspaceManager.WorkspaceExists(stringPtrValue(sameTargetEnvironment.WorkspaceLocator))
				if existsErr != nil {
					return fmt.Errorf("check prepared workspace existence: %w", existsErr)
				}
				if workspaceExists {
					return w.completeOperationRequestTx(ctx, q, operationRequest, &sameTargetEnvironment.ID, operations.OutcomeAlreadyPrepared, operations.StepStatus{
						Name:  operationRequest.CurrentStep,
						State: operations.StepStateCompleted,
					}, map[string]any{
						"runtime_environment_id": sameTargetEnvironment.ID,
						"resolution":             operations.OutcomeAlreadyPrepared,
						"workspace_locator":      stringPtrValue(sameTargetEnvironment.WorkspaceLocator),
					})
				}
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("lookup runtime environment by target: %w", err)
		}

		activeEnvironments, err := q.ListActiveRuntimeEnvironmentsByPullRequestAndType(ctx, sqlc.ListActiveRuntimeEnvironmentsByPullRequestAndTypeParams{
			PullRequestID: operationRequest.PullRequestID,
			Type:          runtimeEnvironmentType,
		})
		if err != nil {
			return fmt.Errorf("list active runtime environments: %w", err)
		}

		runtimeEnvironment, err := q.InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
			RepositoryID:             operationRequest.RepositoryID,
			PullRequestID:            operationRequest.PullRequestID,
			Type:                     runtimeEnvironmentType,
			Status:                   operations.RuntimeStatusPreparing,
			TargetPrHeadCommitSha:    operationRequest.TargetPrHeadCommitSha,
			SourceGithubRepositoryID: snapshot.Target.PullRequestSourceRepo.GithubRepositoryID,
			SourceOwner:              snapshot.Target.PullRequestSourceRepo.Owner,
			SourceName:               snapshot.Target.PullRequestSourceRepo.Name,
			SourceFullName:           snapshot.Target.PullRequestSourceRepo.FullName,
		})
		if err != nil {
			return fmt.Errorf("insert runtime environment: %w", err)
		}

		for _, activeEnvironment := range activeEnvironments {
			if activeEnvironment.ID == runtimeEnvironment.ID {
				continue
			}
			if _, err := q.UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
				ID:     activeEnvironment.ID,
				Status: operations.RuntimeStatusSuperseded,
			}); err != nil {
				return fmt.Errorf("mark runtime environment superseded: %w", err)
			}
			if err := w.insertSupersededCleanupOperationRequestTx(ctx, q, tx, operationRequest, activeEnvironment); err != nil {
				return err
			}
		}

		if _, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateInProgress,
		}, &runtimeEnvironment.ID, map[string]any{
			"runtime_environment_id": runtimeEnvironment.ID,
			"workspace_locator":      stringPtrValue(runtimeEnvironment.WorkspaceLocator),
		}); err != nil {
			return err
		}

		materialization = &previewStartMaterialization{RuntimeEnvironment: runtimeEnvironment}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return materialization, nil
}

func (w *OperationRequestWorker) materializePreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	snapshot operations.PreviewStartSnapshot,
	materialization previewStartMaterialization,
) error {
	repository, err := w.store.Q().GetRepositoryByID(ctx, operationRequest.RepositoryID)
	if err != nil {
		return fmt.Errorf("load repository: %w", err)
	}

	tarballBody, err := w.githubDownloader.DownloadTarball(
		ctx,
		snapshot.Target.PullRequestSourceRepo.Owner,
		snapshot.Target.PullRequestSourceRepo.Name,
		repository.GithubInstallationID,
		snapshot.Target.TargetPRHeadCommitSHA,
	)
	if err != nil {
		return fmt.Errorf("download tarball: %w", err)
	}
	defer tarballBody.Close()

	workspaceLocator := stringPtrValue(materialization.RuntimeEnvironment.WorkspaceLocator)
	stagingPath, err := w.workspaceManager.CreateStaging(workspaceLocator)
	if err != nil {
		return fmt.Errorf("create staging workspace: %w", err)
	}
	defer func() {
		_ = w.workspaceManager.CleanupStaging(stagingPath)
	}()

	if err := w.workspaceManager.ExtractTarball(stagingPath, tarballBody); err != nil {
		return fmt.Errorf("extract tarball: %w", err)
	}

	currentRuntimeEnvironment, err := w.store.Q().GetRuntimeEnvironmentByID(ctx, materialization.RuntimeEnvironment.ID)
	if err != nil {
		return fmt.Errorf("reload runtime environment before promote: %w", err)
	}
	if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
		return w.handleSupersededPreviewStart(ctx, operationRequest, currentRuntimeEnvironment)
	}

	if err := w.workspaceManager.PromoteStaging(stagingPath, workspaceLocator); err != nil {
		return fmt.Errorf("promote staging workspace: %w", err)
	}
	stagingPath = ""

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		currentRuntimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, materialization.RuntimeEnvironment.ID)
		if err != nil {
			return fmt.Errorf("reload runtime environment after promote: %w", err)
		}
		if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
			return w.completeOperationRequestTx(ctx, q, operationRequest, &currentRuntimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
				Name:  operationRequest.CurrentStep,
				State: operations.StepStateFailed,
			}, map[string]any{
				"runtime_environment_id": currentRuntimeEnvironment.ID,
				"reason":                 "runtime_environment_superseded",
				"workspace_locator":      stringPtrValue(currentRuntimeEnvironment.WorkspaceLocator),
			})
		}

		if _, err := q.UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
			ID:     currentRuntimeEnvironment.ID,
			Status: operations.RuntimeStatusPrepared,
		}); err != nil {
			return fmt.Errorf("mark runtime environment prepared: %w", err)
		}

		return w.completeOperationRequestTx(ctx, q, operationRequest, &currentRuntimeEnvironment.ID, operations.OutcomeNewAttemptCreated, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateCompleted,
		}, map[string]any{
			"runtime_environment_id": currentRuntimeEnvironment.ID,
			"workspace_locator":      stringPtrValue(currentRuntimeEnvironment.WorkspaceLocator),
		})
	})
}

func (w *OperationRequestWorker) handleSupersededPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		return w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateFailed,
		}, map[string]any{
			"runtime_environment_id": runtimeEnvironment.ID,
			"reason":                 "runtime_environment_superseded",
			"workspace_locator":      stringPtrValue(runtimeEnvironment.WorkspaceLocator),
		})
	})
}

func (w *OperationRequestWorker) handlePreviewCleanupSuperseded(ctx context.Context, operationRequest sqlc.OperationRequests) error {
	if operationRequest.TargetRuntimeEnvironmentID == nil {
		return errors.New("preview-cleanup-superseded operation is missing target_runtime_environment_id")
	}

	runtimeEnvironment, err := w.store.Q().GetRuntimeEnvironmentByID(ctx, *operationRequest.TargetRuntimeEnvironmentID)
	if err != nil {
		return fmt.Errorf("load cleanup target runtime environment: %w", err)
	}

	if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		_, err := w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateInProgress,
		}, &runtimeEnvironment.ID, map[string]any{
			"runtime_environment_id": runtimeEnvironment.ID,
			"workspace_locator":      stringPtrValue(runtimeEnvironment.WorkspaceLocator),
		})
		return err
	}); err != nil {
		return err
	}

	if err := w.workspaceManager.CleanupWorkspace(stringPtrValue(runtimeEnvironment.WorkspaceLocator)); err != nil {
		return err
	}

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		return w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeCleanupCompleted, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateCompleted,
		}, map[string]any{
			"runtime_environment_id": runtimeEnvironment.ID,
			"workspace_locator":      stringPtrValue(runtimeEnvironment.WorkspaceLocator),
		})
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

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
