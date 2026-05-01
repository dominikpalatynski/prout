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
)

func (s *Server) mount(r chi.Router) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Post("/webhooks/github", s.handleGitHubWebhook)

	r.Route("/api", func(r chi.Router) {
		r.Use(s.requireOperatorBearer)
		r.Get("/trigger-types", s.listTriggerTypes)
		r.Get("/repositories", s.listRepositories)
		r.Post("/repositories", s.registerRepository)
		r.Patch("/repositories/{repositoryID}", s.patchRepository)
		r.Get("/repositories/{repositoryID}/triggers", s.listRepositoryTriggers)
		r.Post("/repositories/{repositoryID}/triggers", s.upsertRepositoryTrigger)
		r.Patch("/repositories/{repositoryID}/triggers/{triggerID}", s.patchRepositoryTrigger)
		r.Get("/webhook-events", s.listWebhookEvents)
		r.Get("/webhook-events/{webhookEventID}", s.getWebhookEvent)
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
