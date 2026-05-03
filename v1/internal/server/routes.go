package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riverqueue/river"

	applog "github.com/dominikpalatynski/toolshed/internal/log"
)

func (s *Server) mount(r chi.Router) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(applog.RequestLogger(s.logger))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Post("/webhooks/github", s.handleGitHubWebhook)

	r.Route("/api", func(r chi.Router) {
		protected := r.With(s.requireOperatorBearer)
		protected.Get("/event-families", s.listEventFamilies)
		protected.Get("/trigger-types", s.listTriggerTypes)
		protected.Get("/repositories", s.listRepositories)
		protected.Post("/repositories", s.registerRepository)
		protected.Patch("/repositories/{repositoryID}", s.patchRepository)
		protected.Get("/repositories/{repositoryID}/event-families", s.listRepositoryEventFamilies)
		protected.Put("/repositories/{repositoryID}/event-families/{eventFamilyKey}", s.putRepositoryEventFamily)
		protected.Patch("/repositories/{repositoryID}/event-families/{eventFamilyKey}", s.patchRepositoryEventFamily)
		protected.Get("/repositories/{repositoryID}/runtime-settings", s.getRepositoryRuntimeSettings)
		protected.Put("/repositories/{repositoryID}/runtime-settings", s.putRepositoryRuntimeSettings)
		protected.Get("/repositories/{repositoryID}/environment-variables", s.listRepositoryEnvironmentVariables)
		protected.Put("/repositories/{repositoryID}/environment-variables", s.putRepositoryEnvironmentVariables)
		protected.Get("/repositories/{repositoryID}/triggers", s.listRepositoryTriggers)
		protected.Post("/repositories/{repositoryID}/triggers", s.upsertRepositoryTrigger)
		protected.Patch("/repositories/{repositoryID}/triggers/{triggerID}", s.patchRepositoryTrigger)
		protected.Get("/runtime-environments", s.listRuntimeEnvironments)
		protected.Get("/operation-requests/{operationRequestID}/history", s.listOperationRequestHistory)
		protected.Get("/operation-requests/{operationRequestID}/history/live", s.streamOperationRequestHistory)
		protected.Get("/webhook-events", s.listWebhookEvents)
		protected.Get("/webhook-events/{webhookEventID}", s.getWebhookEvent)
	})
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.checkReady(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) checkReady(ctx context.Context) error {
	if !s.riverReady.Load() {
		return errors.New("river client has not started")
	}
	if err := s.pool.Ping(ctx); err != nil {
		return err
	}
	if _, err := s.riverClient.QueueList(ctx, river.NewQueueListParams().First(1)); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
