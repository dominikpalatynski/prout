package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/lmittmann/tint"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Config struct {
	Level     slog.Level
	Format    Format
	AddSource bool
	Output    io.Writer
}

func New(cfg Config) *slog.Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	var base slog.Handler
	switch cfg.Format {
	case FormatText:
		base = tint.NewHandler(cfg.Output, &tint.Options{
			Level:      cfg.Level,
			AddSource:  cfg.AddSource,
			TimeFormat: "15:04:05.000",
		})
	default:
		base = slog.NewJSONHandler(cfg.Output, &slog.HandlerOptions{
			Level:     cfg.Level,
			AddSource: cfg.AddSource,
		})
	}
	return slog.New(&ctxHandler{Handler: base})
}

func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type ctxHandler struct{ slog.Handler }

func (h *ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := middleware.GetReqID(ctx); reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	if pr, ok := ctx.Value(prNumberKey{}).(int); ok {
		r.AddAttrs(slog.Int("pr_number", pr))
	}
	if repo, ok := ctx.Value(repoIDKey{}).(int64); ok {
		r.AddAttrs(slog.Int64("repository_id", repo))
	}
	if repo, ok := ctx.Value(githubRepoIDKey{}).(int64); ok {
		r.AddAttrs(slog.Int64("github_repository_id", repo))
	}
	if jobID, ok := ctx.Value(jobIDKey{}).(int64); ok {
		r.AddAttrs(slog.Int64("river_job_id", jobID))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ctxHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *ctxHandler) WithGroup(name string) slog.Handler {
	return &ctxHandler{Handler: h.Handler.WithGroup(name)}
}

type (
	prNumberKey     struct{}
	repoIDKey       struct{}
	githubRepoIDKey struct{}
	jobIDKey        struct{}
)

func WithPRNumber(ctx context.Context, n int) context.Context {
	AddRequestLogAttrs(ctx, slog.Int("pr_number", n))
	return context.WithValue(ctx, prNumberKey{}, n)
}

func WithRepoID(ctx context.Context, id int64) context.Context {
	AddRequestLogAttrs(ctx, slog.Int64("repository_id", id))
	return context.WithValue(ctx, repoIDKey{}, id)
}

func WithGitHubRepositoryID(ctx context.Context, id int64) context.Context {
	AddRequestLogAttrs(ctx, slog.Int64("github_repository_id", id))
	return context.WithValue(ctx, githubRepoIDKey{}, id)
}

func WithJobID(ctx context.Context, id int64) context.Context {
	AddRequestLogAttrs(ctx, slog.Int64("river_job_id", id))
	return context.WithValue(ctx, jobIDKey{}, id)
}
