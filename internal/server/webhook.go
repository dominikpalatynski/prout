package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/jobs"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
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
	if err != nil {
		if len(delivery.PayloadJSON) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}

		failedEvent, duplicate, persistErr := s.persistFailedVerifiedDelivery(r.Context(), delivery, err.Error())
		if persistErr != nil {
			s.logger.ErrorContext(r.Context(), "persist failed webhook event failed", "error", persistErr)
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

	ctx := r.Context()
	if delivery.Supported {
		ctx = applog.WithRepoID(ctx, delivery.Event.GithubRepositoryID)
		ctx = applog.WithPRNumber(ctx, delivery.Event.PRNumber)
	}

	if !delivery.Supported {
		ignoredEvent, duplicate, err := s.persistIgnoredVerifiedDelivery(ctx, delivery, "unsupported_event_type")
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
		return
	}

	result, err := s.processSupportedVerifiedDelivery(ctx, delivery)
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
		s.logger.ErrorContext(ctx, "supported webhook delivery failed", "error", result.ProcessingError, "webhook_event_id", result.Event.ID)
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
		response["dispatch_count"] = result.DispatchCount
	}

	writeJSON(w, http.StatusAccepted, response)
}

type webhookProcessResult struct {
	Event           sqlc.WebhookEvents
	DispatchCount   int
	Duplicate       bool
	ProcessingError error
}

func (s *Server) persistIgnoredVerifiedDelivery(ctx context.Context, delivery webhook.Delivery, ignoredReason string) (sqlc.WebhookEvents, bool, error) {
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

func (s *Server) persistFailedVerifiedDelivery(ctx context.Context, delivery webhook.Delivery, failureMessage string) (sqlc.WebhookEvents, bool, error) {
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

func (s *Server) processSupportedVerifiedDelivery(ctx context.Context, delivery webhook.Delivery) (webhookProcessResult, error) {
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

		repositoryRecord, err := q.GetRepositoryByGitHubRepositoryID(ctx, delivery.Event.GithubRepositoryID)
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

		triggerRecords, err := q.ListEnabledRepositoryTriggers(ctx, repositoryRecord.ID)
		if err != nil {
			return markFailed(&repositoryID, fmt.Errorf("list enabled repository triggers: %w", err))
		}

		for _, triggerRecord := range triggerRecords {
			evaluation, err := s.triggerCatalog.Evaluate(triggerRecord, delivery.DeliveryID, delivery.Event)
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

			if evaluation.DispatchIntent == nil {
				continue
			}

			dispatchRecord, err := q.InsertTriggerDispatch(ctx, sqlc.InsertTriggerDispatchParams{
				WebhookEventID:                  result.Event.ID,
				WebhookEventTriggerEvaluationID: evaluationRecord.ID,
				RepositoryID:                    repositoryRecord.ID,
				RepositoryTriggerID:             triggerRecord.ID,
				DispatchType:                    evaluation.DispatchIntent.DispatchType,
				Status:                          "queued",
				DispatchPayloadJson:             evaluation.DispatchIntent.PayloadJSON,
			})
			if err != nil {
				return markFailed(&repositoryID, fmt.Errorf("insert trigger dispatch: %w", err))
			}

			if _, err := s.riverClient.InsertTx(ctx, tx, jobs.TriggerDispatchArgs{
				TriggerDispatchID: dispatchRecord.ID,
			}, nil); err != nil {
				return markFailed(&repositoryID, fmt.Errorf("enqueue trigger dispatch job: %w", err))
			}

			result.DispatchCount++
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
