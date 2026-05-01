package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dominikpalatynski/toolshed/internal/config"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/triggers"
)

func TestMountLogsUnauthorizedOperatorRequests(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := applog.New(applog.Config{
		Level:  slog.LevelDebug,
		Format: applog.FormatJSON,
		Output: &output,
	})

	s := &Server{
		cfg: &config.Config{
			Operator: config.OperatorConfig{
				BearerToken: "expected-token",
			},
		},
		logger:         logger,
		triggerCatalog: triggers.NewCatalog(),
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(http.MethodGet, "/api/trigger-types", http.NoBody)
	req.RemoteAddr = "198.51.100.8:1234"

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	record := decodeServerLogRecord(t, &output)
	if got := record["level"]; got != "WARN" {
		t.Fatalf("log level = %v, want WARN", got)
	}
	if got := record["route"]; got != "/api/trigger-types" {
		t.Fatalf("log route = %v, want %q", got, "/api/trigger-types")
	}
	if got := record["status"]; got != float64(http.StatusUnauthorized) {
		t.Fatalf("log status = %v, want %d", got, http.StatusUnauthorized)
	}
	if got := record["request_id"]; got == "" {
		t.Fatalf("log request_id = %v, want non-empty", got)
	}
}

func decodeServerLogRecord(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'})
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1", len(lines))
	}

	var record map[string]any
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	return record
}
