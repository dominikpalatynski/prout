package store

import (
	"context"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type ListWebhookEventsParams struct {
	RepositoryID *int64
	Status       *string
	EventType    *string
	DeliveryID   *string
	Limit        int32
}

type WebhookEventDetail struct {
	Event             sqlc.WebhookEvents
	Evaluations       []sqlc.WebhookEventTriggerEvaluations
	OperationRequests []WebhookEventOperationRequestDetail
}

type WebhookEventOperationRequestDetail struct {
	OperationRequest   sqlc.OperationRequests
	RuntimeEnvironment *sqlc.RuntimeEnvironments
}

func (s *Store) ListWebhookEvents(ctx context.Context, params ListWebhookEventsParams) ([]sqlc.WebhookEvents, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}

	const query = `
SELECT
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at
FROM webhook_events
WHERE ($1::bigint IS NULL OR repository_id = $1)
  AND ($2::text IS NULL OR status = $2)
  AND ($3::text IS NULL OR event_type = $3)
  AND ($4::text IS NULL OR delivery_id = $4)
ORDER BY id DESC
LIMIT $5
`

	rows, err := s.pool.Query(ctx, query,
		params.RepositoryID,
		params.Status,
		params.EventType,
		params.DeliveryID,
		params.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []sqlc.WebhookEvents
	for rows.Next() {
		var event sqlc.WebhookEvents
		if err := rows.Scan(
			&event.ID,
			&event.DeliveryID,
			&event.GithubEvent,
			&event.EventType,
			&event.RepositoryID,
			&event.GithubRepositoryID,
			&event.Status,
			&event.IgnoredReason,
			&event.FailureMessage,
			&event.PayloadJson,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s *Store) GetWebhookEventDetail(ctx context.Context, webhookEventID int64) (WebhookEventDetail, error) {
	event, err := s.queries.GetWebhookEventByID(ctx, webhookEventID)
	if err != nil {
		return WebhookEventDetail{}, err
	}

	evaluations, err := s.queries.ListWebhookEventTriggerEvaluationsByWebhookEventID(ctx, webhookEventID)
	if err != nil {
		return WebhookEventDetail{}, err
	}

	operationRequests, err := s.queries.ListWebhookEventOperationRequests(ctx, &webhookEventID)
	if err != nil {
		return WebhookEventDetail{}, err
	}

	requestDetails := make([]WebhookEventOperationRequestDetail, 0, len(operationRequests))
	for _, operationRequest := range operationRequests {
		detail := WebhookEventOperationRequestDetail{
			OperationRequest: operationRequest,
		}
		if operationRequest.RuntimeEnvironmentID != nil {
			runtimeEnvironment, err := s.queries.GetRuntimeEnvironmentByID(ctx, *operationRequest.RuntimeEnvironmentID)
			if err != nil {
				return WebhookEventDetail{}, err
			}
			detail.RuntimeEnvironment = &runtimeEnvironment
		}
		requestDetails = append(requestDetails, detail)
	}

	return WebhookEventDetail{
		Event:             event,
		Evaluations:       evaluations,
		OperationRequests: requestDetails,
	}, nil
}
