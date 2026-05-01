package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dominikpalatynski/toolshed/internal/githubapp"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

func (s *Server) listTriggerTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"trigger_types": s.triggerCatalog.Definitions(),
	})
}

func (s *Server) registerRepository(w http.ResponseWriter, r *http.Request) {
	var request struct {
		FullName string `json:"full_name"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	repository, err := s.githubResolver.ResolveRepository(r.Context(), request.FullName)
	if err != nil {
		var apiErr *githubapp.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			s.logger.ErrorContext(r.Context(), "resolve github repository failed", "error", err, "full_name", request.FullName)
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository not found or GitHub App installation is unavailable for that repository",
			})
			return
		}

		s.logger.ErrorContext(r.Context(), "resolve github repository failed", "error", err, "full_name", request.FullName)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "failed to resolve repository via GitHub",
		})
		return
	}

	record, err := s.store.Q().UpsertRepository(r.Context(), sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   repository.GithubRepositoryID,
		GithubInstallationID: repository.GithubInstallationID,
		Owner:                repository.Owner,
		Name:                 repository.Name,
		FullName:             repository.FullName,
		HtmlUrl:              repository.HTMLURL,
		IsPrivate:            repository.IsPrivate,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "upsert repository failed", "error", err, "full_name", request.FullName)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist repository",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repository": repositoryResponseFromModel(record),
	})
}

func (s *Server) listRepositories(w http.ResponseWriter, r *http.Request) {
	repositories, err := s.store.Q().ListRepositories(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list repositories failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list repositories",
		})
		return
	}

	response := make([]repositoryResponse, 0, len(repositories))
	for _, repository := range repositories {
		response = append(response, repositoryResponseFromModel(repository))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": response,
	})
}

func (s *Server) patchRepository(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if request.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "enabled is required",
		})
		return
	}

	record, err := s.store.Q().SetRepositoryEnabled(r.Context(), sqlc.SetRepositoryEnabledParams{
		ID:      repositoryID,
		Enabled: *request.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository not found",
			})
			return
		}

		s.logger.ErrorContext(r.Context(), "set repository enabled failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update repository",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repository": repositoryResponseFromModel(record),
	})
}

func (s *Server) listRepositoryTriggers(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if _, err := s.store.Q().GetRepositoryByID(r.Context(), repositoryID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository not found",
			})
			return
		}
		s.logger.ErrorContext(r.Context(), "get repository failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load repository",
		})
		return
	}

	triggers, err := s.store.Q().ListRepositoryTriggers(r.Context(), repositoryID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list repository triggers failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list repository triggers",
		})
		return
	}

	response := make([]triggerResponse, 0, len(triggers))
	for _, trigger := range triggers {
		response = append(response, triggerResponseFromModel(trigger))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"triggers": response,
	})
}

func (s *Server) upsertRepositoryTrigger(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if _, err := s.store.Q().GetRepositoryByID(r.Context(), repositoryID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository not found",
			})
			return
		}
		s.logger.ErrorContext(r.Context(), "get repository failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load repository",
		})
		return
	}

	var request struct {
		Type    string          `json:"type"`
		Config  json.RawMessage `json:"config"`
		Enabled *bool           `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	validatedTrigger, err := s.triggerCatalog.ValidateAndNormalize(request.Type, request.Config)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}

	record, err := s.store.Q().UpsertRepositoryTrigger(r.Context(), sqlc.UpsertRepositoryTriggerParams{
		RepositoryID: repositoryID,
		Type:         validatedTrigger.Type,
		EventFamily:  validatedTrigger.EventFamily,
		IdentityKey:  validatedTrigger.IdentityKey,
		ConfigJson:   validatedTrigger.ConfigJSON,
		Enabled:      enabled,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "upsert repository trigger failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist repository trigger",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trigger": triggerResponseFromModel(record),
	})
}

func (s *Server) patchRepositoryTrigger(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	triggerID, err := pathInt64(r, "triggerID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if request.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "enabled is required",
		})
		return
	}

	record, err := s.store.Q().SetRepositoryTriggerEnabled(r.Context(), sqlc.SetRepositoryTriggerEnabledParams{
		RepositoryID: repositoryID,
		ID:           triggerID,
		Enabled:      *request.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository trigger not found",
			})
			return
		}

		s.logger.ErrorContext(r.Context(), "set repository trigger enabled failed", "error", err, "repository_id", repositoryID, "trigger_id", triggerID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update repository trigger",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trigger": triggerResponseFromModel(record),
	})
}

func (s *Server) listWebhookEvents(w http.ResponseWriter, r *http.Request) {
	limit, err := queryLimit(r, 50, 200)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	filters := store.ListWebhookEventsParams{
		Limit: int32(limit),
	}

	if value := strings.TrimSpace(r.URL.Query().Get("repository_id")); value != "" {
		repositoryID, err := strconv.ParseInt(value, 10, 64)
		if err != nil || repositoryID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "repository_id must be a positive integer",
			})
			return
		}
		filters.RepositoryID = &repositoryID
	}

	if value := strings.TrimSpace(r.URL.Query().Get("status")); value != "" {
		if !isAllowed(value, "ignored", "processed", "failed") {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "status must be one of ignored, processed, failed",
			})
			return
		}
		filters.Status = &value
	}

	if value := strings.TrimSpace(r.URL.Query().Get("event_type")); value != "" {
		filters.EventType = &value
	}

	if value := strings.TrimSpace(r.URL.Query().Get("delivery_id")); value != "" {
		filters.DeliveryID = &value
	}

	events, err := s.store.ListWebhookEvents(r.Context(), filters)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list webhook events failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list webhook events",
		})
		return
	}

	response := make([]webhookEventResponse, 0, len(events))
	for _, event := range events {
		response = append(response, webhookEventResponseFromModel(event, false))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"webhook_events": response,
	})
}

func (s *Server) getWebhookEvent(w http.ResponseWriter, r *http.Request) {
	webhookEventID, err := pathInt64(r, "webhookEventID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	detail, err := s.store.GetWebhookEventDetail(r.Context(), webhookEventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "webhook event not found",
			})
			return
		}
		s.logger.ErrorContext(r.Context(), "get webhook event detail failed", "error", err, "webhook_event_id", webhookEventID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load webhook event detail",
		})
		return
	}

	evaluations := make([]webhookEventTriggerEvaluationResponse, 0, len(detail.Evaluations))
	for _, evaluation := range detail.Evaluations {
		evaluations = append(evaluations, webhookEventTriggerEvaluationResponseFromModel(evaluation))
	}

	dispatches := make([]triggerDispatchResponse, 0, len(detail.Dispatches))
	for _, dispatchRecord := range detail.Dispatches {
		dispatches = append(dispatches, triggerDispatchResponseFromModel(dispatchRecord))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"webhook_event": webhookEventResponseFromModel(detail.Event, true),
		"evaluations":   evaluations,
		"dispatches":    dispatches,
	})
}

type repositoryResponse struct {
	ID                   int64     `json:"id"`
	GithubRepositoryID   int64     `json:"github_repository_id"`
	GithubInstallationID int64     `json:"github_installation_id"`
	Owner                string    `json:"owner"`
	Name                 string    `json:"name"`
	FullName             string    `json:"full_name"`
	HTMLURL              string    `json:"html_url"`
	IsPrivate            bool      `json:"is_private"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type triggerResponse struct {
	ID           int64           `json:"id"`
	RepositoryID int64           `json:"repository_id"`
	Type         string          `json:"type"`
	EventFamily  string          `json:"event_family"`
	IdentityKey  string          `json:"identity_key"`
	Config       json.RawMessage `json:"config"`
	Enabled      bool            `json:"enabled"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type webhookEventResponse struct {
	ID                 int64           `json:"id"`
	DeliveryID         string          `json:"delivery_id"`
	GithubEvent        string          `json:"github_event"`
	EventType          string          `json:"event_type"`
	RepositoryID       *int64          `json:"repository_id,omitempty"`
	GithubRepositoryID *int64          `json:"github_repository_id,omitempty"`
	Status             string          `json:"status"`
	IgnoredReason      *string         `json:"ignored_reason,omitempty"`
	FailureMessage     *string         `json:"failure_message,omitempty"`
	PayloadJSON        json.RawMessage `json:"payload_json,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

type webhookEventTriggerEvaluationResponse struct {
	ID                  int64           `json:"id"`
	WebhookEventID      int64           `json:"webhook_event_id"`
	RepositoryTriggerID int64           `json:"repository_trigger_id"`
	Matched             bool            `json:"matched"`
	Reason              string          `json:"reason"`
	TriggerSnapshotJSON json.RawMessage `json:"trigger_snapshot_json"`
	CreatedAt           time.Time       `json:"created_at"`
}

type triggerDispatchResponse struct {
	ID                              int64           `json:"id"`
	WebhookEventID                  int64           `json:"webhook_event_id"`
	WebhookEventTriggerEvaluationID int64           `json:"webhook_event_trigger_evaluation_id"`
	RepositoryID                    int64           `json:"repository_id"`
	RepositoryTriggerID             int64           `json:"repository_trigger_id"`
	DispatchType                    string          `json:"dispatch_type"`
	Status                          string          `json:"status"`
	DispatchPayloadJSON             json.RawMessage `json:"dispatch_payload_json"`
	LastError                       *string         `json:"last_error,omitempty"`
	ProcessedAt                     *time.Time      `json:"processed_at,omitempty"`
	CreatedAt                       time.Time       `json:"created_at"`
}

func repositoryResponseFromModel(record sqlc.Repositories) repositoryResponse {
	return repositoryResponse{
		ID:                   record.ID,
		GithubRepositoryID:   record.GithubRepositoryID,
		GithubInstallationID: record.GithubInstallationID,
		Owner:                record.Owner,
		Name:                 record.Name,
		FullName:             record.FullName,
		HTMLURL:              record.HtmlUrl,
		IsPrivate:            record.IsPrivate,
		Enabled:              record.Enabled,
		CreatedAt:            mustTime(record.CreatedAt),
		UpdatedAt:            mustTime(record.UpdatedAt),
	}
}

func triggerResponseFromModel(record sqlc.RepositoryTriggers) triggerResponse {
	return triggerResponse{
		ID:           record.ID,
		RepositoryID: record.RepositoryID,
		Type:         record.Type,
		EventFamily:  record.EventFamily,
		IdentityKey:  record.IdentityKey,
		Config:       append(json.RawMessage(nil), record.ConfigJson...),
		Enabled:      record.Enabled,
		CreatedAt:    mustTime(record.CreatedAt),
		UpdatedAt:    mustTime(record.UpdatedAt),
	}
}

func webhookEventResponseFromModel(record sqlc.WebhookEvents, includePayload bool) webhookEventResponse {
	response := webhookEventResponse{
		ID:                 record.ID,
		DeliveryID:         record.DeliveryID,
		GithubEvent:        record.GithubEvent,
		EventType:          record.EventType,
		RepositoryID:       record.RepositoryID,
		GithubRepositoryID: record.GithubRepositoryID,
		Status:             record.Status,
		IgnoredReason:      record.IgnoredReason,
		FailureMessage:     record.FailureMessage,
		CreatedAt:          mustTime(record.CreatedAt),
	}
	if includePayload {
		response.PayloadJSON = append(json.RawMessage(nil), record.PayloadJson...)
	}
	return response
}

func webhookEventTriggerEvaluationResponseFromModel(record sqlc.WebhookEventTriggerEvaluations) webhookEventTriggerEvaluationResponse {
	return webhookEventTriggerEvaluationResponse{
		ID:                  record.ID,
		WebhookEventID:      record.WebhookEventID,
		RepositoryTriggerID: record.RepositoryTriggerID,
		Matched:             record.Matched,
		Reason:              record.Reason,
		TriggerSnapshotJSON: append(json.RawMessage(nil), record.TriggerSnapshotJson...),
		CreatedAt:           mustTime(record.CreatedAt),
	}
}

func triggerDispatchResponseFromModel(record sqlc.TriggerDispatches) triggerDispatchResponse {
	var processedAt *time.Time
	if record.ProcessedAt.Valid {
		value := record.ProcessedAt.Time.UTC()
		processedAt = &value
	}

	return triggerDispatchResponse{
		ID:                              record.ID,
		WebhookEventID:                  record.WebhookEventID,
		WebhookEventTriggerEvaluationID: record.WebhookEventTriggerEvaluationID,
		RepositoryID:                    record.RepositoryID,
		RepositoryTriggerID:             record.RepositoryTriggerID,
		DispatchType:                    record.DispatchType,
		Status:                          record.Status,
		DispatchPayloadJSON:             append(json.RawMessage(nil), record.DispatchPayloadJson...),
		LastError:                       record.LastError,
		ProcessedAt:                     processedAt,
		CreatedAt:                       mustTime(record.CreatedAt),
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("request body must contain exactly one JSON object")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain exactly one JSON object")
	}
	return nil
}

func pathInt64(r *http.Request, key string) (int64, error) {
	value := strings.TrimSpace(chi.URLParam(r, key))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func queryLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if parsed > maxValue {
		parsed = maxValue
	}
	return parsed, nil
}

func isAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func mustTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time.UTC()
}
