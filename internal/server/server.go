package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dominikpalatynski/toolshed/internal/config"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	http   *http.Server
}

func New(cfg *config.Config) (*Server, error) {
	logger := applog.New(applog.Config{
		Level:     applog.ParseLevel(cfg.Log.Level),
		Format:    applog.Format(cfg.Log.Format),
		AddSource: cfg.Log.AddSource,
	})

	r := chi.NewRouter()
	s := &Server{cfg: cfg, logger: logger}
	s.mount(r)

	s.http = &http.Server{
		Addr:              cfg.Server.Bind,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.InfoContext(ctx, "server starting", slog.String("addr", s.cfg.Server.Bind))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		s.logger.InfoContext(ctx, "shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
