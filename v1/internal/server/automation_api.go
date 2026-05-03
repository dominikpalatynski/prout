package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

func (s *Server) listEventFamilies(w http.ResponseWriter, _ *http.Request) {
	eventFamilies := s.automationRegistry.EventFamilies()
	response := make([]eventFamilyResponse, 0, len(eventFamilies))
	for _, eventFamily := range eventFamilies {
		response = append(response, eventFamilyResponseFromDefinition(eventFamily))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"event_families": response,
	})
}

func (s *Server) listTriggerTypes(w http.ResponseWriter, _ *http.Request) {
	triggerTypes := s.automationRegistry.TriggerTypes()
	response := make([]triggerTypeResponse, 0, len(triggerTypes))
	for _, triggerType := range triggerTypes {
		response = append(response, triggerTypeResponseFromDefinition(triggerType))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trigger_types": response,
	})
}

func (s *Server) listRepositoryEventFamilies(w http.ResponseWriter, r *http.Request) {
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
	if err := s.ensureRepositoryEventFamilies(ctx, repositoryID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to ensure repository event families",
		})
		return
	}

	records, err := s.store.Q().ListRepositoryEventFamilies(ctx, repositoryID)
	if err != nil {
		s.logger.ErrorContext(ctx, "list repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list repository event families",
		})
		return
	}

	response, err := s.repositoryEventFamilyResponses(records)
	if err != nil {
		s.logger.ErrorContext(ctx, "build repository event family response failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to render repository event families",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"event_families": response,
	})
}

func (s *Server) putRepositoryEventFamily(w http.ResponseWriter, r *http.Request) {
	s.upsertRepositoryEventFamilyEnabled(w, r, true)
}

func (s *Server) patchRepositoryEventFamily(w http.ResponseWriter, r *http.Request) {
	s.upsertRepositoryEventFamilyEnabled(w, r, false)
}

func (s *Server) upsertRepositoryEventFamilyEnabled(w http.ResponseWriter, r *http.Request, replace bool) {
	repositoryID, err := pathInt64(r, "repositoryID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	eventFamilyKey := strings.TrimSpace(chi.URLParam(r, "eventFamilyKey"))
	if eventFamilyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "eventFamilyKey is required",
		})
		return
	}

	ctx := applog.WithRepoID(r.Context(), repositoryID)
	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}

	eventFamilyDefinition, err := s.automationRegistry.EventFamilyByKey(eventFamilyKey)
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

	if err := s.ensureRepositoryEventFamilies(ctx, repositoryID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to ensure repository event families",
		})
		return
	}

	var record sqlc.RepositoryEventFamilies
	if replace {
		record, err = s.store.Q().UpsertRepositoryEventFamily(ctx, sqlc.UpsertRepositoryEventFamilyParams{
			RepositoryID:   repositoryID,
			EventFamilyKey: eventFamilyKey,
			Enabled:        *request.Enabled,
		})
	} else {
		record, err = s.store.Q().SetRepositoryEventFamilyEnabled(ctx, sqlc.SetRepositoryEventFamilyEnabledParams{
			RepositoryID:   repositoryID,
			EventFamilyKey: eventFamilyKey,
			Enabled:        *request.Enabled,
		})
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "repository event family not found",
			})
			return
		}

		s.logger.ErrorContext(
			ctx,
			"persist repository event family failed",
			"error",
			err,
			"repository_id",
			repositoryID,
			"event_family_key",
			eventFamilyKey,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist repository event family",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"event_family": repositoryEventFamilyResponseFromModel(record, eventFamilyDefinition),
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

	ctx := applog.WithRepoID(r.Context(), repositoryID)
	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}
	if err := s.ensureRepositoryEventFamilies(ctx, repositoryID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to ensure repository event families",
		})
		return
	}

	records, err := s.store.Q().ListRepositoryTriggers(ctx, repositoryID)
	if err != nil {
		s.logger.ErrorContext(ctx, "list repository triggers failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list repository triggers",
		})
		return
	}

	response, err := s.repositoryTriggerResponses(ctx, repositoryID, records)
	if err != nil {
		s.logger.ErrorContext(ctx, "build repository trigger response failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to render repository triggers",
		})
		return
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

	ctx := applog.WithRepoID(r.Context(), repositoryID)
	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}
	if err := s.ensureRepositoryEventFamilies(ctx, repositoryID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to ensure repository event families",
		})
		return
	}

	var request struct {
		Type    string `json:"type"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	triggerType, err := s.automationRegistry.TriggerTypeByKey(strings.TrimSpace(request.Type))
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

	record, err := s.store.Q().UpsertRepositoryTrigger(ctx, sqlc.UpsertRepositoryTriggerParams{
		RepositoryID: repositoryID,
		Type:         triggerType.Key,
		Enabled:      enabled,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "upsert repository trigger failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to persist repository trigger",
		})
		return
	}

	response, err := s.repositoryTriggerResponse(ctx, repositoryID, record)
	if err != nil {
		s.logger.ErrorContext(ctx, "build repository trigger response failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to render repository trigger",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trigger": response,
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

	ctx := applog.WithRepoID(r.Context(), repositoryID)
	triggerID, err := pathInt64(r, "triggerID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if _, err := s.requireRepository(ctx, repositoryID); err != nil {
		s.writeRepositoryLookupError(w, ctx, repositoryID, err)
		return
	}
	if err := s.ensureRepositoryEventFamilies(ctx, repositoryID); err != nil {
		s.logger.ErrorContext(ctx, "ensure repository event families failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to ensure repository event families",
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

	record, err := s.store.Q().SetRepositoryTriggerEnabled(ctx, sqlc.SetRepositoryTriggerEnabledParams{
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

		s.logger.ErrorContext(ctx, "set repository trigger enabled failed", "error", err, "repository_id", repositoryID, "trigger_id", triggerID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update repository trigger",
		})
		return
	}

	response, err := s.repositoryTriggerResponse(ctx, repositoryID, record)
	if err != nil {
		s.logger.ErrorContext(ctx, "build repository trigger response failed", "error", err, "repository_id", repositoryID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to render repository trigger",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trigger": response,
	})
}

func (s *Server) ensureRepositoryEventFamilies(ctx context.Context, repositoryID int64) error {
	return s.store.Q().EnsureRepositoryEventFamilies(ctx, sqlc.EnsureRepositoryEventFamiliesParams{
		RepositoryID:    repositoryID,
		EventFamilyKeys: s.automationRegistry.SupportedEventFamilyKeys(),
	})
}

func (s *Server) repositoryEventFamilyResponses(
	records []sqlc.RepositoryEventFamilies,
) ([]repositoryEventFamilyResponse, error) {
	response := make([]repositoryEventFamilyResponse, 0, len(records))
	for _, record := range records {
		eventFamilyDefinition, err := s.automationRegistry.EventFamilyByKey(record.EventFamilyKey)
		if err != nil {
			return nil, err
		}
		response = append(response, repositoryEventFamilyResponseFromModel(record, eventFamilyDefinition))
	}
	return response, nil
}

func (s *Server) repositoryTriggerResponses(
	ctx context.Context,
	repositoryID int64,
	records []sqlc.RepositoryTriggers,
) ([]triggerResponse, error) {
	eventFamilyEnabled, err := s.loadRepositoryEventFamilyEnabledMap(ctx, repositoryID)
	if err != nil {
		return nil, err
	}

	response := make([]triggerResponse, 0, len(records))
	for _, record := range records {
		item, err := s.repositoryTriggerResponseWithEventFamilies(record, eventFamilyEnabled)
		if err != nil {
			return nil, err
		}
		response = append(response, item)
	}
	return response, nil
}

func (s *Server) repositoryTriggerResponse(
	ctx context.Context,
	repositoryID int64,
	record sqlc.RepositoryTriggers,
) (triggerResponse, error) {
	eventFamilyEnabled, err := s.loadRepositoryEventFamilyEnabledMap(ctx, repositoryID)
	if err != nil {
		return triggerResponse{}, err
	}
	return s.repositoryTriggerResponseWithEventFamilies(record, eventFamilyEnabled)
}

func (s *Server) repositoryTriggerResponseWithEventFamilies(
	record sqlc.RepositoryTriggers,
	eventFamilyEnabled map[string]bool,
) (triggerResponse, error) {
	triggerType, err := s.automationRegistry.TriggerTypeByKey(record.Type)
	if err != nil {
		return triggerResponse{}, err
	}
	eventFamily, err := s.automationRegistry.EventFamilyByKey(triggerType.EventFamilyKey)
	if err != nil {
		return triggerResponse{}, err
	}
	return triggerResponseFromModel(record, triggerType, eventFamily, eventFamilyEnabled[triggerType.EventFamilyKey]), nil
}

func (s *Server) loadRepositoryEventFamilyEnabledMap(
	ctx context.Context,
	repositoryID int64,
) (map[string]bool, error) {
	records, err := s.store.Q().ListRepositoryEventFamilies(ctx, repositoryID)
	if err != nil {
		return nil, err
	}

	enabled := make(map[string]bool, len(records))
	for _, record := range records {
		enabled[record.EventFamilyKey] = record.Enabled
	}
	return enabled, nil
}

type eventFamilyResponse struct {
	Key                 string                          `json:"key"`
	Name                string                          `json:"name"`
	Description         string                          `json:"description"`
	GithubEventPatterns []automation.GitHubEventPattern `json:"github_event_patterns"`
	TriggerTypes        []triggerTypeResponse           `json:"trigger_types"`
}

type repositoryEventFamilyResponse struct {
	ID             int64     `json:"id"`
	RepositoryID   int64     `json:"repository_id"`
	EventFamilyKey string    `json:"event_family_key"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type triggerTypeResponse struct {
	Type           string `json:"type"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	EventFamilyKey string `json:"event_family_key"`
	OperationType  string `json:"operation_type"`
}

type triggerResponse struct {
	ID                 int64     `json:"id"`
	RepositoryID       int64     `json:"repository_id"`
	Type               string    `json:"type"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	EventFamilyKey     string    `json:"event_family_key"`
	EventFamilyName    string    `json:"event_family_name"`
	EventFamilyEnabled bool      `json:"event_family_enabled"`
	OperationType      string    `json:"operation_type"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func eventFamilyResponseFromDefinition(definition automation.EventFamilyDefinition) eventFamilyResponse {
	triggerTypes := make([]triggerTypeResponse, 0, len(definition.TriggerTypes))
	for _, triggerType := range definition.TriggerTypes {
		triggerTypes = append(triggerTypes, triggerTypeResponseFromDefinition(triggerType))
	}
	return eventFamilyResponse{
		Key:                 definition.Key,
		Name:                definition.Name,
		Description:         definition.Description,
		GithubEventPatterns: definition.Recognizes,
		TriggerTypes:        triggerTypes,
	}
}

func repositoryEventFamilyResponseFromModel(
	record sqlc.RepositoryEventFamilies,
	definition automation.EventFamilyDefinition,
) repositoryEventFamilyResponse {
	return repositoryEventFamilyResponse{
		ID:             record.ID,
		RepositoryID:   record.RepositoryID,
		EventFamilyKey: record.EventFamilyKey,
		Name:           definition.Name,
		Description:    definition.Description,
		Enabled:        record.Enabled,
		CreatedAt:      mustTime(record.CreatedAt),
		UpdatedAt:      mustTime(record.UpdatedAt),
	}
}

func triggerTypeResponseFromDefinition(definition automation.TriggerTypeDefinition) triggerTypeResponse {
	return triggerTypeResponse{
		Type:           definition.Key,
		Name:           definition.Name,
		Description:    definition.Description,
		EventFamilyKey: definition.EventFamilyKey,
		OperationType:  definition.StartsOperation,
	}
}

func triggerResponseFromModel(
	record sqlc.RepositoryTriggers,
	triggerType automation.TriggerTypeDefinition,
	eventFamily automation.EventFamilyDefinition,
	eventFamilyEnabled bool,
) triggerResponse {
	return triggerResponse{
		ID:                 record.ID,
		RepositoryID:       record.RepositoryID,
		Type:               record.Type,
		Name:               triggerType.Name,
		Description:        triggerType.Description,
		EventFamilyKey:     triggerType.EventFamilyKey,
		EventFamilyName:    eventFamily.Name,
		EventFamilyEnabled: eventFamilyEnabled,
		OperationType:      triggerType.StartsOperation,
		Enabled:            record.Enabled,
		CreatedAt:          mustTime(record.CreatedAt),
		UpdatedAt:          mustTime(record.UpdatedAt),
	}
}
