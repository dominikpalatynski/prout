package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riverqueue/river"

	"github.com/dominikpalatynski/toolshed/internal/jobs"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func (s *Server) mount(r chi.Router) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Get("/debug/pings", s.debugPings)
	r.Post("/webhooks/github", s.handleGitHubWebhook)
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

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	args, decision, err := webhook.NormalizeGitHubDelivery(
		r.Header.Get("X-GitHub-Event"),
		r.Header.Get("X-GitHub-Delivery"),
		http.MaxBytesReader(w, r.Body, 1<<20),
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if decision == webhook.DecisionIgnore {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
		return
	}

	ctx := applog.WithRepoID(r.Context(), args.RepositoryID)
	ctx = applog.WithPRNumber(ctx, args.PRNumber)

	jobID, err := s.enqueueRecordPing(ctx, args)
	if err != nil {
		s.logger.ErrorContext(ctx, "enqueue record ping job failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to enqueue record ping job",
		})
		return
	}

	s.logger.InfoContext(ctx, "accepted github webhook",
		"delivery_id", args.DeliveryID,
		"river_job_id", jobID,
	)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":       "accepted",
		"river_job_id": jobID,
	})
}

func (s *Server) debugPings(w http.ResponseWriter, r *http.Request) {
	limit := int32(20)
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = int32(parsed)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	pings, err := s.store.Q().ListPings(ctx, limit)
	if err != nil {
		s.logger.ErrorContext(ctx, "list pings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list pings",
		})
		return
	}

	type pingResponse struct {
		ID       int64     `json:"id"`
		PingedAt time.Time `json:"pinged_at"`
	}

	response := make([]pingResponse, 0, len(pings))
	for _, ping := range pings {
		if !ping.PingedAt.Valid {
			continue
		}
		response = append(response, pingResponse{
			ID:       ping.ID,
			PingedAt: ping.PingedAt.Time.UTC(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"limit": limit,
		"pings": response,
	})
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

func (s *Server) enqueueRecordPing(ctx context.Context, args jobs.RecordPingArgs) (int64, error) {
	result, err := s.riverClient.Insert(ctx, args, nil)
	if err != nil {
		return 0, err
	}
	return result.Job.ID, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
