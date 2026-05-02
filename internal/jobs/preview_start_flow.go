package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	runtimebackend "github.com/dominikpalatynski/toolshed/internal/runtime"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

const runtimeDeploymentTeardownTimeout = 2 * time.Minute

func (w *OperationRequestWorker) handlePreviewStart(ctx context.Context, operationRequest sqlc.OperationRequests) error {
	snapshot, err := operations.ParsePreviewStartSnapshot(operationRequest.IntentSnapshotJson)
	if err != nil {
		return err
	}
	if !snapshot.Target.PullRequestSourceRepo.IsComplete() {
		return errors.New("preview-start snapshot is missing pull-request source repository details")
	}

	if operationRequest.RuntimeEnvironmentID == nil {
		completed, err := w.startPreviewStart(ctx, operationRequest, snapshot)
		if err != nil || completed {
			return err
		}
	}

	return w.runPreviewStartPipeline(ctx, operationRequest.ID, snapshot)
}

func (w *OperationRequestWorker) startPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	snapshot operations.PreviewStartSnapshot,
) (bool, error) {
	ctx = w.withHistoryAfterCommit(ctx)

	completed := false

	err := w.store.Tx(ctx, func(q *sqlc.Queries, tx pgx.Tx) error {
		operationDefinition, err := w.automationRegistry.OperationByKey(operationRequest.OperationType)
		if err != nil {
			return err
		}
		runtimeEnvironmentType := operationDefinition.RuntimeEnvironmentType

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
				completed = true
				return w.completeOperationRequestTx(ctx, q, operationRequest, &sameTargetEnvironment.ID, operations.OutcomeAlreadyPreparing, operations.StepStatus{
					Name:  operationRequest.CurrentStep,
					State: operations.StepStateCompleted,
				}, runtimeEnvironmentStepDetails(sameTargetEnvironment, map[string]any{
					"resolution": operations.OutcomeAlreadyPreparing,
				}))
			case operations.RuntimeStatusPrepared:
				artifactsExist, err := w.preparedArtifactsExistForReuse(ctx, sameTargetEnvironment)
				if err != nil {
					return err
				}
				if artifactsExist {
					completed = true
					return w.completeOperationRequestTx(ctx, q, operationRequest, &sameTargetEnvironment.ID, operations.OutcomeAlreadyPrepared, operations.StepStatus{
						Name:  operationRequest.CurrentStep,
						State: operations.StepStateCompleted,
					}, runtimeEnvironmentStepDetails(sameTargetEnvironment, map[string]any{
						"resolution": operations.OutcomeAlreadyPrepared,
					}))
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

		_, err = w.transitionOperationRequestStepTx(ctx, q, operationRequest, operations.StepStatus{
			Name:  operations.StepSourceMaterialization,
			State: operations.StepStateInProgress,
		}, &runtimeEnvironment.ID, runtimeEnvironmentStepDetails(runtimeEnvironment, nil))
		return err
	})
	if err != nil {
		return false, err
	}

	return completed, nil
}

func (w *OperationRequestWorker) runPreviewStartPipeline(
	ctx context.Context,
	operationRequestID int64,
	snapshot operations.PreviewStartSnapshot,
) error {
	for {
		operationRequest, err := w.store.Q().GetOperationRequestByID(ctx, operationRequestID)
		if err != nil {
			return fmt.Errorf("reload operation request: %w", err)
		}
		if operationRequest.Status == operations.StatusHandled {
			return nil
		}
		if operationRequest.RuntimeEnvironmentID == nil {
			return errors.New("preview-start operation is missing runtime_environment_id")
		}

		runtimeEnvironment, err := w.store.Q().GetRuntimeEnvironmentByID(ctx, *operationRequest.RuntimeEnvironmentID)
		if err != nil {
			return fmt.Errorf("load runtime environment: %w", err)
		}

		switch runtimeEnvironment.Status {
		case operations.RuntimeStatusPrepared:
			if err := w.completePreparedPreviewStart(ctx, operationRequest, runtimeEnvironment); err != nil {
				return err
			}
			continue
		case operations.RuntimeStatusSuperseded:
			return w.handleSupersededPreviewStart(ctx, operationRequest, runtimeEnvironment)
		case operations.RuntimeStatusPreparing:
			// Keep going.
		default:
			return runtimebackend.PermanentError(fmt.Errorf(
				"runtime environment %d has unsupported status %q",
				runtimeEnvironment.ID,
				runtimeEnvironment.Status,
			))
		}

		switch operationRequest.CurrentStep {
		case operations.StepSourceMaterialization:
			if err := w.runSourceMaterializationStep(ctx, operationRequest, snapshot, runtimeEnvironment); err != nil {
				return err
			}
		case operations.StepComposePreparation:
			if err := w.runComposePreparationStep(ctx, operationRequest, runtimeEnvironment); err != nil {
				return err
			}
		case operations.StepRuntimeDeployment:
			if err := w.runRuntimeDeploymentStep(ctx, operationRequest, runtimeEnvironment); err != nil {
				return err
			}
		default:
			return runtimebackend.PermanentError(fmt.Errorf(
				"preview-start operation is at unsupported step %q",
				operationRequest.CurrentStep,
			))
		}
	}
}

func (w *OperationRequestWorker) completePreparedPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	artifactsExist, err := w.preparedArtifactsExistForReuse(ctx, runtimeEnvironment)
	if err != nil {
		return err
	}
	if !artifactsExist {
		return runtimebackend.PermanentError(fmt.Errorf(
			"prepared runtime environment %d is missing deployment artifacts",
			runtimeEnvironment.ID,
		))
	}

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		return w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeNewAttemptCreated, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateCompleted,
		}, runtimeEnvironmentStepDetails(runtimeEnvironment, map[string]any{
			"resolution": operations.OutcomeNewAttemptCreated,
		}))
	})
}

func (w *OperationRequestWorker) runSourceMaterializationStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	snapshot operations.PreviewStartSnapshot,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	currentRequest := operationRequest
	if currentRequest.CurrentStepState == operations.StepStatePending {
		if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
			updated, err := w.transitionOperationRequestStepTx(ctx, q, currentRequest, operations.StepStatus{
				Name:  operations.StepSourceMaterialization,
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

	workspaceLocator := stringPtrValue(runtimeEnvironment.WorkspaceLocator)
	workspaceExists, err := w.workspaceManager.WorkspaceExists(workspaceLocator)
	if err != nil {
		return fmt.Errorf("check workspace existence: %w", err)
	}
	if workspaceExists {
		return w.advanceSourceMaterializationStep(ctx, currentRequest, runtimeEnvironment)
	}

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

	currentRuntimeEnvironment, err := w.store.Q().GetRuntimeEnvironmentByID(ctx, runtimeEnvironment.ID)
	if err != nil {
		return fmt.Errorf("reload runtime environment before promote: %w", err)
	}
	if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
		return w.handleSupersededPreviewStart(ctx, currentRequest, currentRuntimeEnvironment)
	}

	if err := w.workspaceManager.PromoteStaging(stagingPath, workspaceLocator); err != nil {
		return fmt.Errorf("promote staging workspace: %w", err)
	}
	stagingPath = ""

	return w.advanceSourceMaterializationStep(ctx, currentRequest, currentRuntimeEnvironment)
}

func (w *OperationRequestWorker) advanceSourceMaterializationStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		currentRuntimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, runtimeEnvironment.ID)
		if err != nil {
			return fmt.Errorf("reload runtime environment after materialization: %w", err)
		}
		if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
			return w.completeOperationRequestTx(ctx, q, operationRequest, &currentRuntimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
				Name:  operationRequest.CurrentStep,
				State: operations.StepStateFailed,
			}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
				"reason": "runtime_environment_superseded",
			}))
		}

		_, err = w.advanceOperationRequestStepTx(ctx, q, operationRequest, &currentRuntimeEnvironment.ID, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, nil), operations.StepStatus{
			Name:  operations.StepComposePreparation,
			State: operations.StepStatePending,
		}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, nil))
		return err
	})
}

func (w *OperationRequestWorker) runComposePreparationStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	currentRequest := operationRequest
	if currentRequest.CurrentStepState == operations.StepStatePending {
		if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
			updated, err := w.transitionOperationRequestStepTx(ctx, q, currentRequest, operations.StepStatus{
				Name:  operations.StepComposePreparation,
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

	workspace, err := w.openRuntimeWorkspace(runtimeEnvironment)
	if err != nil {
		return err
	}

	configuration, err := w.store.GetRepositoryRuntimeConfiguration(ctx, runtimeEnvironment.RepositoryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return runtimebackend.PermanentError(fmt.Errorf(
				"repository runtime settings are missing for repository %d",
				runtimeEnvironment.RepositoryID,
			))
		}
		return err
	}

	deploymentRecord, err := w.runtimeBackend.Prepare(ctx, runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: runtimeEnvironment.ID,
		RepositoryID:         runtimeEnvironment.RepositoryID,
		PullRequestID:        runtimeEnvironment.PullRequestID,
		TargetCommitSHA:      runtimeEnvironment.TargetPrHeadCommitSha,
		Workspace:            workspace,
		RuntimeSettings:      runtimeSettingsFromModel(configuration.RuntimeSettings),
		EnvironmentVariables: environmentVariablesFromModels(configuration.EnvironmentVariables),
	})
	if err != nil {
		return err
	}

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		currentRuntimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, runtimeEnvironment.ID)
		if err != nil {
			return fmt.Errorf("reload runtime environment after preparation: %w", err)
		}
		if _, err := q.UpsertRuntimeEnvironmentDeployment(ctx, sqlc.UpsertRuntimeEnvironmentDeploymentParams{
			RuntimeEnvironmentID:           currentRuntimeEnvironment.ID,
			Backend:                        deploymentRecord.Backend,
			FrozenRuntimeSettingsJson:      deploymentRecord.FrozenRuntimeSettingsJSON,
			FrozenEnvironmentVariablesJson: deploymentRecord.FrozenEnvironmentVariablesJSON,
			DeploymentMetadataJson:         deploymentRecord.MetadataJSON,
		}); err != nil {
			return fmt.Errorf("upsert runtime environment deployment: %w", err)
		}
		if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
			return w.completeOperationRequestTx(ctx, q, currentRequest, &currentRuntimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
				Name:  currentRequest.CurrentStep,
				State: operations.StepStateFailed,
			}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
				"reason": "runtime_environment_superseded",
			}))
		}

		_, err = w.advanceOperationRequestStepTx(ctx, q, currentRequest, &currentRuntimeEnvironment.ID, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
			"deployment_backend": deploymentRecord.Backend,
		}), operations.StepStatus{
			Name:  operations.StepRuntimeDeployment,
			State: operations.StepStatePending,
		}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
			"deployment_backend": deploymentRecord.Backend,
		}))
		return err
	})
}

func (w *OperationRequestWorker) runRuntimeDeploymentStep(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	currentRequest := operationRequest
	if currentRequest.CurrentStepState == operations.StepStatePending {
		if err := w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
			updated, err := w.transitionOperationRequestStepTx(ctx, q, currentRequest, operations.StepStatus{
				Name:  operations.StepRuntimeDeployment,
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

	workspace, err := w.openRuntimeWorkspace(runtimeEnvironment)
	if err != nil {
		return err
	}

	deploymentRecord, found, err := w.loadRuntimeEnvironmentDeploymentRecord(ctx, runtimeEnvironment.ID)
	if err != nil {
		return err
	}
	if !found {
		return runtimebackend.PermanentError(fmt.Errorf(
			"runtime environment %d is missing its frozen deployment record",
			runtimeEnvironment.ID,
		))
	}

	if err := w.runtimeBackend.Deploy(ctx, runtimebackend.DeployRequest{
		Workspace:  workspace,
		Deployment: deploymentRecord,
	}); err != nil {
		teardownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeDeploymentTeardownTimeout)
		defer cancel()

		if teardownErr := w.runtimeBackend.Teardown(teardownCtx, runtimebackend.TeardownRequest{
			Workspace:  workspace,
			Deployment: deploymentRecord,
		}); teardownErr != nil {
			w.logger.WarnContext(ctx, "best-effort runtime teardown after failed deploy also failed",
				"runtime_environment_id", runtimeEnvironment.ID,
				"deploy_error", err,
				"teardown_error", teardownErr,
			)
		}
		return err
	}

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		currentRuntimeEnvironment, err := q.GetRuntimeEnvironmentByID(ctx, runtimeEnvironment.ID)
		if err != nil {
			return fmt.Errorf("reload runtime environment after deploy: %w", err)
		}
		if currentRuntimeEnvironment.Status == operations.RuntimeStatusSuperseded {
			return w.completeOperationRequestTx(ctx, q, currentRequest, &currentRuntimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
				Name:  currentRequest.CurrentStep,
				State: operations.StepStateFailed,
			}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
				"reason": "runtime_environment_superseded",
			}))
		}

		if _, err := q.UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
			ID:     currentRuntimeEnvironment.ID,
			Status: operations.RuntimeStatusPrepared,
		}); err != nil {
			return fmt.Errorf("mark runtime environment prepared: %w", err)
		}

		return w.completeOperationRequestTx(ctx, q, currentRequest, &currentRuntimeEnvironment.ID, operations.OutcomeNewAttemptCreated, operations.StepStatus{
			Name:  currentRequest.CurrentStep,
			State: operations.StepStateCompleted,
		}, runtimeEnvironmentStepDetails(currentRuntimeEnvironment, map[string]any{
			"deployment_backend": deploymentRecord.Backend,
		}))
	})
}

func (w *OperationRequestWorker) handleSupersededPreviewStart(
	ctx context.Context,
	operationRequest sqlc.OperationRequests,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) error {
	ctx = w.withHistoryAfterCommit(ctx)

	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		return w.completeOperationRequestTx(ctx, q, operationRequest, &runtimeEnvironment.ID, operations.OutcomeAttemptSuperseded, operations.StepStatus{
			Name:  operationRequest.CurrentStep,
			State: operations.StepStateFailed,
		}, runtimeEnvironmentStepDetails(runtimeEnvironment, map[string]any{
			"reason": "runtime_environment_superseded",
		}))
	})
}

func (w *OperationRequestWorker) preparedArtifactsExistForReuse(
	ctx context.Context,
	runtimeEnvironment sqlc.RuntimeEnvironments,
) (bool, error) {
	workspaceLocator := stringPtrValue(runtimeEnvironment.WorkspaceLocator)
	if workspaceLocator == "" {
		return false, nil
	}

	workspaceExists, err := w.workspaceManager.WorkspaceExists(workspaceLocator)
	if err != nil {
		return false, fmt.Errorf("check workspace existence: %w", err)
	}
	if !workspaceExists {
		return false, nil
	}

	workspace, err := w.workspaceManager.OpenWorkspace(workspaceLocator)
	if err != nil {
		return false, nil
	}

	deploymentRecord, found, err := w.loadRuntimeEnvironmentDeploymentRecord(ctx, runtimeEnvironment.ID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	artifactsExist, err := w.runtimeBackend.PreparedArtifactsExist(ctx, runtimebackend.PreparedArtifactsRequest{
		Workspace:  workspace,
		Deployment: deploymentRecord,
	})
	if err != nil {
		return false, nil
	}

	return artifactsExist, nil
}

func (w *OperationRequestWorker) openRuntimeWorkspace(
	runtimeEnvironment sqlc.RuntimeEnvironments,
) (runtimebackend.Workspace, error) {
	workspaceLocator := stringPtrValue(runtimeEnvironment.WorkspaceLocator)
	if workspaceLocator == "" {
		return nil, runtimebackend.PermanentError(fmt.Errorf(
			"runtime environment %d is missing its workspace locator",
			runtimeEnvironment.ID,
		))
	}

	workspace, err := w.workspaceManager.OpenWorkspace(workspaceLocator)
	if err != nil {
		return nil, runtimebackend.PermanentError(fmt.Errorf(
			"open workspace for runtime environment %d: %w",
			runtimeEnvironment.ID,
			err,
		))
	}

	return workspace, nil
}

func (w *OperationRequestWorker) loadRuntimeEnvironmentDeploymentRecord(
	ctx context.Context,
	runtimeEnvironmentID int64,
) (runtimebackend.DeploymentRecord, bool, error) {
	record, err := w.store.Q().GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID(ctx, runtimeEnvironmentID)
	switch {
	case err == nil:
		return deploymentRecordFromModel(record), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return runtimebackend.DeploymentRecord{}, false, nil
	default:
		return runtimebackend.DeploymentRecord{}, false, fmt.Errorf("load runtime environment deployment: %w", err)
	}
}

func runtimeSettingsFromModel(record sqlc.RepositoryRuntimeSettings) runtimebackend.RuntimeSettings {
	return runtimebackend.RuntimeSettings{
		ComposeFilePath:    stringPtrValue(record.ComposeFilePath),
		ExposedServiceName: stringPtrValue(record.ExposedServiceName),
		ExposedServicePort: int32PtrValue(record.ExposedServicePort),
	}
}

func environmentVariablesFromModels(
	records []sqlc.RepositoryEnvironmentVariables,
) []runtimebackend.EnvironmentVariable {
	variables := make([]runtimebackend.EnvironmentVariable, 0, len(records))
	for _, record := range records {
		variables = append(variables, runtimebackend.EnvironmentVariable{
			Name:  record.Name,
			Value: record.Value,
		})
	}
	return variables
}

func int32PtrValue(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}
