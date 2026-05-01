package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestRequestLoggerLogsCompletionWithEnrichment(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelDebug,
		Format: FormatJSON,
		Output: &output,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(RequestLogger(logger))
	r.Post("/repositories/{repositoryID}/pulls/{prNumber}", func(w http.ResponseWriter, r *http.Request) {
		ctx := WithRepoID(r.Context(), 42)
		ctx = WithGitHubRepositoryID(ctx, 9001)
		ctx = WithPRNumber(ctx, 7)
		AddRequestLogAttrs(ctx, slog.String("delivery_id", "delivery-1"))

		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/repositories/42/pulls/7", http.NoBody)
	req.Header.Set("User-Agent", "toolshed-test")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.RemoteAddr = "198.51.100.8:1234"

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusCreated)
	}

	record := decodeSingleLogRecord(t, &output)
	if got := record["level"]; got != "INFO" {
		t.Fatalf("log level = %v, want INFO", got)
	}
	if got := record["msg"]; got != "http request completed" {
		t.Fatalf("log msg = %v, want %q", got, "http request completed")
	}
	if got := record["method"]; got != http.MethodPost {
		t.Fatalf("log method = %v, want %q", got, http.MethodPost)
	}
	if got := record["route"]; got != "/repositories/{repositoryID}/pulls/{prNumber}" {
		t.Fatalf("log route = %v, want %q", got, "/repositories/{repositoryID}/pulls/{prNumber}")
	}
	if got := record["path"]; got != "/repositories/42/pulls/7" {
		t.Fatalf("log path = %v, want %q", got, "/repositories/42/pulls/7")
	}
	if got := record["status"]; got != float64(http.StatusCreated) {
		t.Fatalf("log status = %v, want %d", got, http.StatusCreated)
	}
	if got := record["bytes_written"]; got != float64(len("ok")) {
		t.Fatalf("log bytes_written = %v, want %d", got, len("ok"))
	}
	if got := record["remote_ip"]; got != "203.0.113.9" {
		t.Fatalf("log remote_ip = %v, want %q", got, "203.0.113.9")
	}
	if got := record["user_agent"]; got != "toolshed-test" {
		t.Fatalf("log user_agent = %v, want %q", got, "toolshed-test")
	}
	if got := record["repository_id"]; got != float64(42) {
		t.Fatalf("log repository_id = %v, want 42", got)
	}
	if got := record["github_repository_id"]; got != float64(9001) {
		t.Fatalf("log github_repository_id = %v, want 9001", got)
	}
	if got := record["pr_number"]; got != float64(7) {
		t.Fatalf("log pr_number = %v, want 7", got)
	}
	if got := record["delivery_id"]; got != "delivery-1" {
		t.Fatalf("log delivery_id = %v, want %q", got, "delivery-1")
	}
	if got := record["request_id"]; got == "" {
		t.Fatalf("log request_id = %v, want non-empty", got)
	}
	if got, ok := record["duration_ms"].(float64); !ok || got < 0 {
		t.Fatalf("log duration_ms = %v, want non-negative number", record["duration_ms"])
	}
}

func TestRequestLoggerLogsRecoveredPanicsAtErrorLevel(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelDebug,
		Format: FormatJSON,
		Output: &output,
	})

	r := chi.NewRouter()
	r.Use(RequestLogger(logger))
	r.Use(testRecoverer)
	r.Get("/boom", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	record := decodeSingleLogRecord(t, &output)
	if got := record["level"]; got != "ERROR" {
		t.Fatalf("log level = %v, want ERROR", got)
	}
	if got := record["route"]; got != "/boom" {
		t.Fatalf("log route = %v, want %q", got, "/boom")
	}
	if got := record["status"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("log status = %v, want %d", got, http.StatusInternalServerError)
	}
}

func testRecoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func decodeSingleLogRecord(t *testing.T, output *bytes.Buffer) map[string]any {
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
