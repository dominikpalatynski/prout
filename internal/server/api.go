package server

import (
	"context"
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
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

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

	githubRepository, err := s.githubResolver.ResolveRepository(r.Context(), request.FullName)
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

	ctx := applog.WithGitHubRepositoryID(r.Context(), githubRepository.GithubRepositoryID)

	record, err := s.store.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   githubRepository.GithubRepositoryID,
		GithubInstallationID: githubRepository.GithubInstallationID,
		Owner:                githubRepository.Owner,
		Name:                 githubRepository.Name,
		FullName:             githubRepository.FullName,
		HtmlUrl:              githubRepository.HTMLURL,
		IsPrivate:            githubRepository.IsPrivate,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "upsert repository failed", "error", err, "full_name", request.FullName)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist repository",
		})
		return
	}

	repositoryRecord := store.RepositoryFromUpsertRow(record)
	if err := s.ensureRepositoryEventFamilies(ctx, repositoryRecord.ID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryRecord.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to seed repository event families",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repository": repositoryResponseFromModel(repositoryRecord),
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

func (s *Server) listRuntimeEnvironments(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := queryInt64(r, "repository_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	prNumber, err := queryInt64(r, "pr_number")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if prNumber != nil && repositoryID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "repository_id is required when pr_number is set",
		})
		return
	}

	items, err := s.store.ListRuntimeEnvironments(r.Context(), store.ListRuntimeEnvironmentsParams{
		RepositoryID: repositoryID,
		PRNumber:     prNumber,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list runtime environments failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list runtime environments",
		})
		return
	}

	response := make([]runtimeEnvironmentListItemResponse, 0, len(items))
	for _, item := range items {
		response = append(response, runtimeEnvironmentListItemResponse{
			RuntimeEnvironment: runtimeEnvironmentResponseFromModel(item.RuntimeEnvironment),
			PullRequest:        pullRequestSummaryResponseFromModel(item.PullRequest),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runtime_environments": response,
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

	ctx := applog.WithRepoID(r.Context(), repositoryID)

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

	record, err := s.store.Q().SetRepositoryEnabled(ctx, sqlc.SetRepositoryEnabledParams{
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

		s.logger.ErrorContext(ctx, "set repository enabled failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update repository",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repository": repositoryResponseFromModel(record),
	})
}

func (s *Server) getRepositoryRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	ctx := applog.WithRepoID(r.Context(), repositoryID)

	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}

	record, err := s.loadOrCreateRepositoryRuntimeSettings(ctx, repositoryID)
	if err != nil {
		s.logger.ErrorContext(ctx, "load repository runtime settings failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load repository runtime settings",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runtime_settings": repositoryRuntimeSettingsResponseFromModel(record),
	})
}

func (s *Server) putRepositoryRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	ctx := applog.WithRepoID(r.Context(), repositoryID)

	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}

	var request struct {
		ComposeFilePath    string `json:"compose_file_path"`
		ExposedServiceName string `json:"exposed_service_name"`
		ExposedServicePort int32  `json:"exposed_service_port"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if request.ExposedServicePort < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "exposed_service_port must be greater than or equal to 0",
		})
		return
	}

	record, err := s.store.Q().UpsertRepositoryRuntimeSettings(ctx, sqlc.UpsertRepositoryRuntimeSettingsParams{
		RepositoryID:       repositoryID,
		ComposeFilePath:    optionalTrimmedString(request.ComposeFilePath),
		ExposedServiceName: optionalTrimmedString(request.ExposedServiceName),
		ExposedServicePort: optionalInt32(request.ExposedServicePort),
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "upsert repository runtime settings failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update repository runtime settings",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runtime_settings": repositoryRuntimeSettingsResponseFromModel(record),
	})
}

func (s *Server) listRepositoryEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	ctx := applog.WithRepoID(r.Context(), repositoryID)

	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}

	records, err := s.store.Q().ListRepositoryEnvironmentVariablesByRepositoryID(ctx, repositoryID)
	if err != nil {
		s.logger.ErrorContext(ctx, "list repository environment variables failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list repository environment variables",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"environment_variables": repositoryEnvironmentVariableResponsesFromModels(records),
	})
}

func (s *Server) putRepositoryEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	ctx := applog.WithRepoID(r.Context(), repositoryID)

	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}

	var request struct {
		EnvironmentVariables []repositoryEnvironmentVariableRequest `json:"environment_variables"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	inputs := make([]store.RepositoryEnvironmentVariableInput, 0, len(request.EnvironmentVariables))
	for _, variable := range request.EnvironmentVariables {
		inputs = append(inputs, store.RepositoryEnvironmentVariableInput{
			Name:  variable.Name,
			Value: variable.Value,
		})
	}

	records, err := s.store.ReplaceRepositoryEnvironmentVariables(ctx, repositoryID, inputs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"environment_variables": repositoryEnvironmentVariableResponsesFromModels(records),
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

	ctx := r.Context()
	if filters.RepositoryID != nil {
		ctx = applog.WithRepoID(ctx, *filters.RepositoryID)
	}

	events, err := s.store.ListWebhookEvents(ctx, filters)
	if err != nil {
		s.logger.ErrorContext(ctx, "list webhook events failed", "error", err)
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

	includePayload, err := queryBool(r, "include_payload", false)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	includeSnapshots, err := queryBool(r, "include_snapshots", false)
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
		evaluations = append(evaluations, webhookEventTriggerEvaluationResponseFromModel(evaluation, includeSnapshots))
	}

	operationRequests := make([]operationRequestResponse, 0, len(detail.OperationRequests))
	for _, operationRequest := range detail.OperationRequests {
		operationRequests = append(operationRequests, operationRequestResponseFromDetail(operationRequest, includeSnapshots))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"webhook_event":      webhookEventResponseFromModel(detail.Event, includePayload),
		"evaluations":        evaluations,
		"operation_requests": operationRequests,
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

type repositoryRuntimeSettingsResponse struct {
	RepositoryID       int64     `json:"repository_id"`
	ComposeFilePath    *string   `json:"compose_file_path,omitempty"`
	ExposedServiceName *string   `json:"exposed_service_name,omitempty"`
	ExposedServicePort *int32    `json:"exposed_service_port,omitempty"`
	Complete           bool      `json:"complete"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type repositoryEnvironmentVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type repositoryEnvironmentVariableResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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
	TriggerSnapshotJSON json.RawMessage `json:"trigger_snapshot_json,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}

type pullRequestSourceRepositoryResponse struct {
	GithubRepositoryID int64  `json:"github_repository_id"`
	Owner              string `json:"owner"`
	Name               string `json:"name"`
	FullName           string `json:"full_name"`
}

type pullRequestSummaryResponse struct {
	ID                      int64                               `json:"id"`
	RepositoryID            int64                               `json:"repository_id"`
	PRNumber                int64                               `json:"pr_number"`
	GithubPullRequestID     *int64                              `json:"github_pull_request_id,omitempty"`
	CurrentHeadCommitSHA    string                              `json:"current_head_commit_sha"`
	CurrentSourceRepository pullRequestSourceRepositoryResponse `json:"current_source_repository"`
	CreatedAt               time.Time                           `json:"created_at"`
	UpdatedAt               time.Time                           `json:"updated_at"`
}

type runtimeEnvironmentResponse struct {
	ID                    int64                               `json:"id"`
	RepositoryID          int64                               `json:"repository_id"`
	PullRequestID         int64                               `json:"pull_request_id"`
	Type                  string                              `json:"type"`
	Status                string                              `json:"status"`
	TargetPRHeadCommitSHA string                              `json:"target_pr_head_commit_sha"`
	SourceRepository      pullRequestSourceRepositoryResponse `json:"source_repository"`
	WorkspaceLocator      string                              `json:"workspace_locator"`
	CreatedAt             time.Time                           `json:"created_at"`
	UpdatedAt             time.Time                           `json:"updated_at"`
}

type runtimeEnvironmentListItemResponse struct {
	RuntimeEnvironment runtimeEnvironmentResponse `json:"runtime_environment"`
	PullRequest        pullRequestSummaryResponse `json:"pull_request"`
}

type operationRequestResponse struct {
	ID                              int64                       `json:"id"`
	WebhookEventID                  *int64                      `json:"webhook_event_id,omitempty"`
	WebhookEventTriggerEvaluationID *int64                      `json:"webhook_event_trigger_evaluation_id,omitempty"`
	RepositoryID                    int64                       `json:"repository_id"`
	RepositoryTriggerID             *int64                      `json:"repository_trigger_id,omitempty"`
	PullRequestID                   int64                       `json:"pull_request_id"`
	RuntimeEnvironmentID            *int64                      `json:"runtime_environment_id,omitempty"`
	TargetRuntimeEnvironmentID      *int64                      `json:"target_runtime_environment_id,omitempty"`
	OperationType                   string                      `json:"operation_type"`
	Source                          string                      `json:"source"`
	Status                          string                      `json:"status"`
	TargetPRHeadCommitSHA           string                      `json:"target_pr_head_commit_sha"`
	IntentSnapshotJSON              json.RawMessage             `json:"intent_snapshot_json,omitempty"`
	CurrentStep                     string                      `json:"current_step"`
	CurrentStepState                string                      `json:"current_step_state"`
	CurrentStepDetailsJSON          json.RawMessage             `json:"current_step_details_json,omitempty"`
	Outcome                         *string                     `json:"outcome,omitempty"`
	LastError                       *string                     `json:"last_error,omitempty"`
	HandledAt                       *time.Time                  `json:"handled_at,omitempty"`
	CreatedAt                       time.Time                   `json:"created_at"`
	RuntimeEnvironment              *runtimeEnvironmentResponse `json:"runtime_environment,omitempty"`
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

func repositoryRuntimeSettingsResponseFromModel(
	record sqlc.RepositoryRuntimeSettings,
) repositoryRuntimeSettingsResponse {
	return repositoryRuntimeSettingsResponse{
		RepositoryID:       record.RepositoryID,
		ComposeFilePath:    record.ComposeFilePath,
		ExposedServiceName: record.ExposedServiceName,
		ExposedServicePort: record.ExposedServicePort,
		Complete:           repositoryRuntimeSettingsComplete(record),
		CreatedAt:          mustTime(record.CreatedAt),
		UpdatedAt:          mustTime(record.UpdatedAt),
	}
}

func repositoryEnvironmentVariableResponsesFromModels(
	records []sqlc.RepositoryEnvironmentVariables,
) []repositoryEnvironmentVariableResponse {
	response := make([]repositoryEnvironmentVariableResponse, 0, len(records))
	for _, record := range records {
		response = append(response, repositoryEnvironmentVariableResponse{
			Name:  record.Name,
			Value: record.Value,
		})
	}
	return response
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

func webhookEventTriggerEvaluationResponseFromModel(record sqlc.WebhookEventTriggerEvaluations, includeSnapshot bool) webhookEventTriggerEvaluationResponse {
	response := webhookEventTriggerEvaluationResponse{
		ID:                  record.ID,
		WebhookEventID:      record.WebhookEventID,
		RepositoryTriggerID: record.RepositoryTriggerID,
		Matched:             record.Matched,
		Reason:              record.Reason,
		CreatedAt:           mustTime(record.CreatedAt),
	}
	if includeSnapshot {
		response.TriggerSnapshotJSON = append(json.RawMessage(nil), record.TriggerSnapshotJson...)
	}
	return response
}

func pullRequestSourceRepositoryResponseFromValues(
	githubRepositoryID int64,
	owner, name, fullName string,
) pullRequestSourceRepositoryResponse {
	return pullRequestSourceRepositoryResponse{
		GithubRepositoryID: githubRepositoryID,
		Owner:              owner,
		Name:               name,
		FullName:           fullName,
	}
}

func pullRequestSummaryResponseFromModel(record sqlc.PullRequests) pullRequestSummaryResponse {
	return pullRequestSummaryResponse{
		ID:                   record.ID,
		RepositoryID:         record.RepositoryID,
		PRNumber:             record.PRNumber,
		GithubPullRequestID:  record.GithubPullRequestID,
		CurrentHeadCommitSHA: record.CurrentHeadCommitSha,
		CurrentSourceRepository: pullRequestSourceRepositoryResponseFromValues(
			record.CurrentSourceGithubRepositoryID,
			record.CurrentSourceOwner,
			record.CurrentSourceName,
			record.CurrentSourceFullName,
		),
		CreatedAt: mustTime(record.CreatedAt),
		UpdatedAt: mustTime(record.UpdatedAt),
	}
}

func runtimeEnvironmentResponseFromModel(record sqlc.RuntimeEnvironments) runtimeEnvironmentResponse {
	return runtimeEnvironmentResponse{
		ID:                    record.ID,
		RepositoryID:          record.RepositoryID,
		PullRequestID:         record.PullRequestID,
		Type:                  record.Type,
		Status:                record.Status,
		TargetPRHeadCommitSHA: record.TargetPrHeadCommitSha,
		SourceRepository: pullRequestSourceRepositoryResponseFromValues(
			record.SourceGithubRepositoryID,
			record.SourceOwner,
			record.SourceName,
			record.SourceFullName,
		),
		WorkspaceLocator: stringPtrValue(record.WorkspaceLocator),
		CreatedAt:        mustTime(record.CreatedAt),
		UpdatedAt:        mustTime(record.UpdatedAt),
	}
}

func operationRequestResponseFromDetail(detail store.WebhookEventOperationRequestDetail, includeSnapshot bool) operationRequestResponse {
	record := detail.OperationRequest

	var handledAt *time.Time
	if record.HandledAt.Valid {
		value := record.HandledAt.Time.UTC()
		handledAt = &value
	}

	response := operationRequestResponse{
		ID:                              record.ID,
		WebhookEventID:                  record.WebhookEventID,
		WebhookEventTriggerEvaluationID: record.WebhookEventTriggerEvaluationID,
		RepositoryID:                    record.RepositoryID,
		RepositoryTriggerID:             record.RepositoryTriggerID,
		PullRequestID:                   record.PullRequestID,
		RuntimeEnvironmentID:            record.RuntimeEnvironmentID,
		TargetRuntimeEnvironmentID:      record.TargetRuntimeEnvironmentID,
		OperationType:                   record.OperationType,
		Source:                          record.Source,
		Status:                          record.Status,
		TargetPRHeadCommitSHA:           record.TargetPrHeadCommitSha,
		CurrentStep:                     record.CurrentStep,
		CurrentStepState:                record.CurrentStepState,
		Outcome:                         record.Outcome,
		LastError:                       record.LastError,
		HandledAt:                       handledAt,
		CreatedAt:                       mustTime(record.CreatedAt),
	}
	if includeSnapshot {
		response.IntentSnapshotJSON = append(json.RawMessage(nil), record.IntentSnapshotJson...)
	}
	if len(record.CurrentStepDetailsJson) > 0 {
		response.CurrentStepDetailsJSON = append(json.RawMessage(nil), record.CurrentStepDetailsJson...)
	}
	if detail.RuntimeEnvironment != nil {
		runtimeEnvironment := runtimeEnvironmentResponseFromModel(*detail.RuntimeEnvironment)
		response.RuntimeEnvironment = &runtimeEnvironment
	}
	return response
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

func queryBool(r *http.Request, key string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func queryInt64(r *http.Request, key string) (*int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", key)
	}
	return &parsed, nil
}

func isAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (s *Server) requireRepository(ctx context.Context, repositoryID int64) (sqlc.Repositories, error) {
	return s.store.Q().GetRepositoryByID(ctx, repositoryID)
}

func (s *Server) writeRepositoryLookupError(
	w http.ResponseWriter,
	ctx context.Context,
	repositoryID int64,
	err error,
) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "repository not found",
		})
		return
	}

	s.logger.ErrorContext(ctx, "get repository failed", "error", err, "repository_id", repositoryID)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "failed to load repository",
	})
}

func (s *Server) loadOrCreateRepositoryRuntimeSettings(
	ctx context.Context,
	repositoryID int64,
) (sqlc.RepositoryRuntimeSettings, error) {
	record, err := s.store.Q().GetRepositoryRuntimeSettingsByRepositoryID(ctx, repositoryID)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.RepositoryRuntimeSettings{}, err
	}

	return s.store.Q().UpsertRepositoryRuntimeSettings(ctx, sqlc.UpsertRepositoryRuntimeSettingsParams{
		RepositoryID: repositoryID,
	})
}

func repositoryRuntimeSettingsComplete(record sqlc.RepositoryRuntimeSettings) bool {
	return strings.TrimSpace(stringPtrValue(record.ComposeFilePath)) != "" &&
		strings.TrimSpace(stringPtrValue(record.ExposedServiceName)) != "" &&
		int32PtrValue(record.ExposedServicePort) > 0
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalInt32(value int32) *int32 {
	if value == 0 {
		return nil
	}
	return &value
}

func int32PtrValue(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func mustTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time.UTC()
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
