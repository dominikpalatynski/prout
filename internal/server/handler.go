package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/dominikpalatynski/toolshed/internal/github"
)

func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received readyz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready"})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received healthz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "healthy"})
}

func (s *Server) testDownloadHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received test download request")
	// s.queue.Run(func() error {
	// 	return s.workspaceHandler.HandleCreateWorkspace()
	// })
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "download has been queued"})
}

func (s *Server) handleGithubWebhook(w http.ResponseWriter, r *http.Request) {
	var payload github.PullRequestWebhookPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		slog.Error("failed to decode github webhook payload", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := s.githubEventHandler.HandleGithubEvent(payload); err != nil {
		slog.Error("failed to validate github webhook payload", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "download has been queued"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("Failed to write JSON response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
