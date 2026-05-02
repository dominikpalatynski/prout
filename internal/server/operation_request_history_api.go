package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type operationRequestHistoryEntryResponse struct {
	ID                 int64           `json:"id"`
	OperationRequestID int64           `json:"operation_request_id"`
	Kind               string          `json:"kind"`
	Step               *string         `json:"step,omitempty"`
	Message            string          `json:"message"`
	DetailsJSON        json.RawMessage `json:"details_json,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

func (s *Server) listOperationRequestHistory(w http.ResponseWriter, r *http.Request) {
	operationRequestID, err := pathInt64(r, "operationRequestID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if _, err := s.store.Q().GetOperationRequestByID(r.Context(), operationRequestID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "operation request not found",
			})
			return
		}

		s.logger.ErrorContext(r.Context(), "load operation request failed", "error", err, "operation_request_id", operationRequestID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load operation request",
		})
		return
	}

	entries, err := s.store.ListOperationRequestHistoryEntries(r.Context(), operationRequestID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list operation request history failed", "error", err, "operation_request_id", operationRequestID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load operation request history",
		})
		return
	}

	response := make([]operationRequestHistoryEntryResponse, 0, len(entries))
	for _, entry := range entries {
		response = append(response, operationRequestHistoryEntryResponseFromModel(entry))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"operation_request_history_entries": response,
	})
}

func (s *Server) streamOperationRequestHistory(w http.ResponseWriter, r *http.Request) {
	operationRequestID, err := pathInt64(r, "operationRequestID")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	if _, err := s.store.Q().GetOperationRequestByID(r.Context(), operationRequestID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "operation request not found",
			})
			return
		}

		s.logger.ErrorContext(r.Context(), "load operation request failed", "error", err, "operation_request_id", operationRequestID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load operation request",
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "streaming is unavailable",
		})
		return
	}
	if s.operationRequestHistoryHub == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "history streaming is unavailable",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	entries, unsubscribe := s.operationRequestHistoryHub.Subscribe(operationRequestID)
	defer unsubscribe()

	fmt.Fprint(w, ": operation request history stream connected\n\n")
	flusher.Flush()

	if err := streamOperationRequestHistoryEntries(r.Context(), w, flusher, entries); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.ErrorContext(r.Context(), "stream operation request history failed", "error", err, "operation_request_id", operationRequestID)
	}
}

func streamOperationRequestHistoryEntries(
	ctx context.Context,
	w http.ResponseWriter,
	flusher http.Flusher,
	entries <-chan sqlc.OperationRequestHistoryEntries,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-entries:
			if !ok {
				return nil
			}

			payload, err := json.Marshal(operationRequestHistoryEntryResponseFromModel(entry))
			if err != nil {
				return fmt.Errorf("marshal operation request history entry: %w", err)
			}

			if _, err := fmt.Fprintf(w, "id: %d\nevent: operation_request_history_entry\ndata: %s\n\n", entry.ID, payload); err != nil {
				return fmt.Errorf("write operation request history stream: %w", err)
			}
			flusher.Flush()
		}
	}
}

func operationRequestHistoryEntryResponseFromModel(
	entry sqlc.OperationRequestHistoryEntries,
) operationRequestHistoryEntryResponse {
	response := operationRequestHistoryEntryResponse{
		ID:                 entry.ID,
		OperationRequestID: entry.OperationRequestID,
		Kind:               entry.Kind,
		Step:               entry.Step,
		Message:            entry.Message,
		CreatedAt:          mustTime(entry.CreatedAt),
	}
	if len(entry.DetailsJson) > 0 {
		response.DetailsJSON = append(json.RawMessage(nil), entry.DetailsJson...)
	}
	return response
}
