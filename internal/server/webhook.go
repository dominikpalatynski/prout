package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	"github.com/dominikpalatynski/toolshed/internal/jobs"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read webhook payload",
		})
		return
	}

	if err := webhook.VerifySignature(s.cfg.GitHub.WebhookSecret, r.Header.Get("X-Hub-Signature-256"), body); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": err.Error(),
		})
		return
	}

	delivery, err := webhook.ParseDelivery(
		r.Header.Get("X-GitHub-Event"),
		r.Header.Get("X-GitHub-Delivery"),
		body,
	)
	ctx := r.Context()
	if delivery.GithubRepositoryID != nil {
		ctx = applog.WithGitHubRepositoryID(ctx, *delivery.GithubRepositoryID)
	}
	if err != nil {
		if len(delivery.PayloadJSON) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}

		failedEvent, duplicate, persistErr := s.persistFailedVerifiedDelivery(ctx, delivery, err.Error())
		if persistErr != nil {
			s.logger.ErrorContext(ctx, "persist failed webhook event failed", "error", persistErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to persist webhook event",
			})
			return
		}
		if duplicate {
			writeJSON(w, http.StatusOK, map[string]string{
				"status": "duplicate",
			})
			return
		}

		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status":           "failed",
			"webhook_event_id": failedEvent.ID,
			"error":            err.Error(),
		})
		return
	}

	eventFamily, supported := s.automationRegistry.ResolveEventFamily(delivery)
	if !supported {
		s.writeIgnoredWebhookResponse(w, ctx, delivery, "unsupported_event_type")
		return
	}

	event, applicable, err := eventFamily.Normalize(delivery)
	if err != nil {
		failedEvent, duplicate, persistErr := s.persistFailedVerifiedDelivery(ctx, delivery, err.Error())
		if persistErr != nil {
			s.logger.ErrorContext(ctx, "persist failed webhook event failed", "error", persistErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to persist webhook event",
			})
			return
		}
		if duplicate {
			writeJSON(w, http.StatusOK, map[string]string{
				"status": "duplicate",
			})
			return
		}

		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status":           "failed",
			"webhook_event_id": failedEvent.ID,
			"error":            err.Error(),
		})
		return
	}
	if !applicable {
		s.writeIgnoredWebhookResponse(w, ctx, delivery, "unsupported_event_type")
		return
	}

	ctx = applog.WithPRNumber(ctx, event.PRNumber)

	result, err := s.processSupportedVerifiedDelivery(ctx, delivery, eventFamily, event)
	if result.Event.RepositoryID != nil {
		ctx = applog.WithRepoID(ctx, *result.Event.RepositoryID)
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "process webhook delivery failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to process webhook event",
		})
		return
	}
	if result.Duplicate {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "duplicate",
		})
		return
	}
	if result.ProcessingError != nil {
		s.logger.ErrorContext(
			ctx,
			"supported webhook delivery failed",
			"error",
			result.ProcessingError,
			"webhook_event_id",
			result.Event.ID,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":           result.Event.Status,
			"webhook_event_id": result.Event.ID,
			"error":            result.ProcessingError.Error(),
		})
		return
	}

	response := map[string]any{
		"status":           result.Event.Status,
		"webhook_event_id": result.Event.ID,
	}
	if result.Event.IgnoredReason != nil {
		response["ignored_reason"] = *result.Event.IgnoredReason
	}
	if result.Event.Status == "processed" {
		response["operation_request_count"] = result.OperationRequestCount
	}

	writeJSON(w, http.StatusAccepted, response)
}

type webhookProcessResult struct {
	Event                 sqlc.WebhookEvents
	OperationRequestCount int
	Duplicate             bool
	ProcessingError       error
}

func (s *Server) writeIgnoredWebhookResponse(
	w http.ResponseWriter,
	ctx context.Context,
	delivery webhook.Delivery,
	ignoredReason string,
) {
	ignoredEvent, duplicate, err := s.persistIgnoredVerifiedDelivery(ctx, delivery, ignoredReason)
	if err != nil {
		s.logger.ErrorContext(ctx, "persist ignored webhook event failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist webhook event",
		})
		return
	}
	if duplicate {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "duplicate",
		})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":           ignoredEvent.Status,
		"webhook_event_id": ignoredEvent.ID,
		"ignored_reason":   ignoredEvent.IgnoredReason,
	})
}

func (s *Server) persistIgnoredVerifiedDelivery(
	ctx context.Context,
	delivery webhook.Delivery,
	ignoredReason string,
) (sqlc.WebhookEvents, bool, error) {
	var (
		event     sqlc.WebhookEvents
		duplicate bool
	)

	err := s.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		inserted, err := q.InsertWebhookEvent(ctx, sqlc.InsertWebhookEventParams{
			DeliveryID:         delivery.DeliveryID,
			GithubEvent:        delivery.GithubEvent,
			EventType:          delivery.EventType,
			GithubRepositoryID: delivery.GithubRepositoryID,
			Status:             "ignored",
			PayloadJson:        delivery.PayloadJSON,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				duplicate = true
				return nil
			}
			return err
		}

		event, err = q.MarkWebhookEventIgnored(ctx, sqlc.MarkWebhookEventIgnoredParams{
			ID:            inserted.ID,
			RepositoryID:  nil,
			IgnoredReason: &ignoredReason,
		})
		return err
	})

	return event, duplicate, err
}

func (s *Server) persistFailedVerifiedDelivery(
	ctx context.Context,
	delivery webhook.Delivery,
	failureMessage string,
) (sqlc.WebhookEvents, bool, error) {
	var (
		event     sqlc.WebhookEvents
		duplicate bool
	)

	err := s.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		inserted, err := q.InsertWebhookEvent(ctx, sqlc.InsertWebhookEventParams{
			DeliveryID:         delivery.DeliveryID,
			GithubEvent:        delivery.GithubEvent,
			EventType:          delivery.EventType,
			GithubRepositoryID: delivery.GithubRepositoryID,
			Status:             "failed",
			PayloadJson:        delivery.PayloadJSON,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				duplicate = true
				return nil
			}
			return err
		}

		event, err = q.MarkWebhookEventFailed(ctx, sqlc.MarkWebhookEventFailedParams{
			ID:             inserted.ID,
			RepositoryID:   nil,
			FailureMessage: &failureMessage,
		})
		return err
	})

	return event, duplicate, err
}

func (s *Server) processSupportedVerifiedDelivery(
	ctx context.Context,
	delivery webhook.Delivery,
	eventFamily automation.EventFamilyDefinition,
	event webhook.NormalizedEvent,
) (webhookProcessResult, error) {
	var result webhookProcessResult

	err := s.store.Tx(ctx, func(q *sqlc.Queries, tx pgx.Tx) error {
		inserted, err := q.InsertWebhookEvent(ctx, sqlc.InsertWebhookEventParams{
			DeliveryID:         delivery.DeliveryID,
			GithubEvent:        delivery.GithubEvent,
			EventType:          delivery.EventType,
			GithubRepositoryID: delivery.GithubRepositoryID,
			Status:             "failed",
			PayloadJson:        delivery.PayloadJSON,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				result.Duplicate = true
				return nil
			}
			return err
		}

		result.Event = inserted

		markFailed := func(repositoryID *int64, cause error) error {
			result.ProcessingError = cause
			message := cause.Error()
			updated, err := q.MarkWebhookEventFailed(ctx, sqlc.MarkWebhookEventFailedParams{
				ID:             result.Event.ID,
				RepositoryID:   repositoryID,
				FailureMessage: &message,
			})
			if err == nil {
				result.Event = updated
			}
			return err
		}

		repositoryRecord, err := q.GetRepositoryByGitHubRepositoryID(ctx, event.GithubRepositoryID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				ignoredReason := "repository_not_registered"
				updated, updateErr := q.MarkWebhookEventIgnored(ctx, sqlc.MarkWebhookEventIgnoredParams{
					ID:            result.Event.ID,
					RepositoryID:  nil,
					IgnoredReason: &ignoredReason,
				})
				if updateErr != nil {
					return updateErr
				}
				result.Event = updated
				return nil
			}
			return markFailed(nil, fmt.Errorf("lookup repository by github_repository_id: %w", err))
		}

		repositoryID := repositoryRecord.ID
		if !repositoryRecord.Enabled {
			ignoredReason := "repository_disabled"
			updated, updateErr := q.MarkWebhookEventIgnored(ctx, sqlc.MarkWebhookEventIgnoredParams{
				ID:            result.Event.ID,
				RepositoryID:  &repositoryID,
				IgnoredReason: &ignoredReason,
			})
			if updateErr != nil {
				return updateErr
			}
			result.Event = updated
			return nil
		}

		if err := s.ensureRepositoryEventFamiliesWithQueries(ctx, q, repositoryID); err != nil {
			return markFailed(&repositoryID, fmt.Errorf("ensure repository event families: %w", err))
		}

		repositoryEventFamily, err := q.GetRepositoryEventFamily(ctx, sqlc.GetRepositoryEventFamilyParams{
			RepositoryID:   repositoryID,
			EventFamilyKey: eventFamily.Key,
		})
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("load repository event family %q: %w", eventFamily.Key, err))
		}
		if !repositoryEventFamily.Enabled {
			ignoredReason := eventFamily.DisabledReason
			updated, updateErr := q.MarkWebhookEventIgnored(ctx, sqlc.MarkWebhookEventIgnoredParams{
				ID:            result.Event.ID,
				RepositoryID:  &repositoryID,
				IgnoredReason: &ignoredReason,
			})
			if updateErr != nil {
				return updateErr
			}
			result.Event = updated
			return nil
		}

		event, err := s.resolvePullRequestTarget(ctx, repositoryRecord, event)
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("resolve pull request target: %w", err))
		}

		pullRequestRecord, err := q.UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
			RepositoryID:                    repositoryRecord.ID,
			PRNumber:                        int64(event.PRNumber),
			GithubPullRequestID:             githubPullRequestIDParam(event.GithubPullRequestID),
			CurrentHeadCommitSha:            event.PRHeadSHA,
			CurrentSourceGithubRepositoryID: event.PRSourceRepository.GithubRepositoryID,
			CurrentSourceOwner:              event.PRSourceRepository.Owner,
			CurrentSourceName:               event.PRSourceRepository.Name,
			CurrentSourceFullName:           event.PRSourceRepository.FullName,
		})
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("upsert pull request anchor: %w", err))
		}

		triggerRecords, err := q.ListEnabledRepositoryTriggers(ctx, repositoryRecord.ID)
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("list enabled repository triggers: %w", err))
		}

		for _, triggerRecord := range triggerRecords {
			triggerType, err := s.automationRegistry.TriggerTypeByKey(triggerRecord.Type)
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("resolve repository trigger %d: %w", triggerRecord.ID, err))
			}
			if triggerType.EventFamilyKey != eventFamily.Key {
				continue
			}

			evaluation, err := s.automationRegistry.EvaluateRepositoryTrigger(triggerRecord, eventFamily.Key, event)
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("evaluate repository trigger %d: %w", triggerRecord.ID, err))
			}

			evaluationRecord, err := q.InsertWebhookEventTriggerEvaluation(ctx, sqlc.InsertWebhookEventTriggerEvaluationParams{
				WebhookEventID:      result.Event.ID,
				RepositoryTriggerID: triggerRecord.ID,
				Matched:             evaluation.Matched,
				Reason:              evaluation.Reason,
				TriggerSnapshotJson: evaluation.TriggerSnapshotJSON,
			})
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("insert webhook event trigger evaluation: %w", err))
			}

			if !evaluation.Matched {
				continue
			}

			operationDefinition, err := s.automationRegistry.OperationByKey(evaluation.OperationType)
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("resolve operation definition %q: %w", evaluation.OperationType, err))
			}
			if operationDefinition.BuildTriggerIntentSnapshot == nil {
				return markFailed(&repositoryID, fmt.Errorf(
					"operation type %q does not support trigger intent snapshots",
					evaluation.OperationType,
				))
			}

			intentSnapshotJSON, err := operationDefinition.BuildTriggerIntentSnapshot(operations.PreviewStartSnapshotInput{
				RepositoryID:           repositoryRecord.ID,
				PullRequestID:          pullRequestRecord.ID,
				PRNumber:               pullRequestRecord.PRNumber,
				GithubPullRequestID:    pullRequestRecord.GithubPullRequestID,
				PRSourceRepository:     event.PRSourceRepository,
				DeliveryID:             delivery.DeliveryID,
				Event:                  event,
				TriggerID:              triggerRecord.ID,
				TriggerType:            triggerRecord.Type,
				OperationType:          evaluation.OperationType,
				RuntimeEnvironmentType: operationDefinition.RuntimeEnvironmentType,
				TargetPRHeadCommitSHA:  event.PRHeadSHA,
			})
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("build operation request snapshot: %w", err))
			}

			operationRequest, err := q.InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
				WebhookEventID:                  &result.Event.ID,
				WebhookEventTriggerEvaluationID: &evaluationRecord.ID,
				RepositoryID:                    repositoryRecord.ID,
				RepositoryTriggerID:             &triggerRecord.ID,
				PullRequestID:                   pullRequestRecord.ID,
				RuntimeEnvironmentID:            nil,
				TargetRuntimeEnvironmentID:      nil,
				OperationType:                   evaluation.OperationType,
				Source:                          operations.SourceTrigger,
				Status:                          operations.StatusQueued,
				TargetPrHeadCommitSha:           event.PRHeadSHA,
				IntentSnapshotJson:              intentSnapshotJSON,
				CurrentStep:                     operationDefinition.InitialStep.Name,
				CurrentStepState:                operationDefinition.InitialStep.State,
				CurrentStepDetailsJson:          nil,
			})
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("insert operation request: %w", err))
			}

			if _, err := s.riverClient.InsertTx(ctx, tx, jobs.OperationRequestArgs{
				OperationRequestID: operationRequest.ID,
			}, nil); err != nil {
				return markFailed(&repositoryID, fmt.Errorf("enqueue operation request job: %w", err))
			}

			result.OperationRequestCount++
		}

		updated, err := q.MarkWebhookEventProcessed(ctx, sqlc.MarkWebhookEventProcessedParams{
			ID:           result.Event.ID,
			RepositoryID: &repositoryID,
		})
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("mark webhook event processed: %w", err))
		}
		result.Event = updated
		return nil
	})

	return result, err
}

func (s *Server) ensureRepositoryEventFamiliesWithQueries(
	ctx context.Context,
	q *sqlc.Queries,
	repositoryID int64,
) error {
	return q.EnsureRepositoryEventFamilies(ctx, sqlc.EnsureRepositoryEventFamiliesParams{
		RepositoryID:    repositoryID,
		EventFamilyKeys: s.automationRegistry.SupportedEventFamilyKeys(),
	})
}

func (s *Server) resolvePullRequestTarget(
	ctx context.Context,
	repository sqlc.Repositories,
	event webhook.NormalizedEvent,
) (webhook.NormalizedEvent, error) {
	if strings.TrimSpace(event.PRHeadSHA) != "" && event.PRSourceRepository.IsComplete() {
		return event, nil
	}
	if s.githubResolver == nil {
		return webhook.NormalizedEvent{}, errors.New("github resolver is unavailable")
	}

	pullRequest, err := s.githubResolver.ResolvePullRequest(
		ctx,
		repository.Owner,
		repository.Name,
		repository.GithubInstallationID,
		event.PRNumber,
	)
	if err != nil {
		return webhook.NormalizedEvent{}, err
	}

	if event.GithubPullRequestID == 0 {
		event.GithubPullRequestID = pullRequest.GithubPullRequestID
	}
	if strings.TrimSpace(event.PRHeadSHA) == "" {
		event.PRHeadSHA = pullRequest.HeadSHA
	}
	if !event.PRSourceRepository.IsComplete() {
		event.PRSourceRepository = pullRequest.SourceRepository
	}
	return event, nil
}

func githubPullRequestIDParam(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}
