package log

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type requestLogAttrsKey struct{}

type requestLogAttrs struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			collector := &requestLogAttrs{}
			ctx := context.WithValue(r.Context(), requestLogAttrsKey{}, collector)
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("route", routePattern(r.Context())),
				slog.String("path", r.URL.Path),
				slog.Int("status", normalizedStatus(ww.Status())),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.Int("bytes_written", ww.BytesWritten()),
				slog.String("remote_ip", remoteIP(r.RemoteAddr)),
				slog.String("user_agent", r.UserAgent()),
			}

			attrs = append(attrs, collector.snapshot()...)
			logger.LogAttrs(r.Context(), requestLogLevel(ww.Status()), "http request completed", attrs...)
		})
	}
}

func AddRequestLogAttrs(ctx context.Context, attrs ...slog.Attr) {
	if len(attrs) == 0 {
		return
	}

	collector, ok := ctx.Value(requestLogAttrsKey{}).(*requestLogAttrs)
	if !ok || collector == nil {
		return
	}

	collector.add(attrs...)
}

func (c *requestLogAttrs) add(attrs ...slog.Attr) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.attrs = append(c.attrs, attrs...)
}

func (c *requestLogAttrs) snapshot() []slog.Attr {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]slog.Attr(nil), c.attrs...)
}

func requestLogLevel(status int) slog.Level {
	switch {
	case normalizedStatus(status) >= http.StatusInternalServerError:
		return slog.LevelError
	case normalizedStatus(status) >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func normalizedStatus(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func routePattern(ctx context.Context) string {
	routeCtx := chi.RouteContext(ctx)
	if routeCtx == nil {
		return ""
	}
	return routeCtx.RoutePattern()
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
