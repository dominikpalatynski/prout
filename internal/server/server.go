package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/dominikpalatynski/toolshed/internal/config"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/store"
)

type Server struct {
	cfg         *config.Config
	logger      *slog.Logger
	http        *http.Server
	pool        *pgxpool.Pool
	store       *store.Store
	riverClient *river.Client[pgx.Tx]
	riverReady  atomic.Bool
}

func New(cfg *config.Config) (*Server, error) {
	logger := applog.New(applog.Config{
		Level:     applog.ParseLevel(cfg.Log.Level),
		Format:    applog.Format(cfg.Log.Format),
		AddSource: cfg.Log.AddSource,
	})

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, pingStore, riverClient, err := bootstrap(startupCtx, cfg, logger)
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	s := &Server{
		cfg:         cfg,
		logger:      logger,
		pool:        pool,
		store:       pingStore,
		riverClient: riverClient,
	}
	s.mount(r)

	s.http = &http.Server{
		Addr:              cfg.Server.Bind,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	if err := s.riverClient.Start(ctx); err != nil {
		s.pool.Close()
		return fmt.Errorf("start river client: %w", err)
	}
	s.riverReady.Store(true)

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
		return s.shutdown(shutdownCtx)
	case err := <-errCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if shutdownErr := s.shutdown(shutdownCtx); shutdownErr != nil {
			return errors.Join(err, shutdownErr)
		}
		return err
	}
}

func (s *Server) shutdown(ctx context.Context) error {
	var shutdownErr error

	if err := s.http.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown http server: %w", err))
	}

	if s.riverReady.Swap(false) {
		if err := s.riverClient.Stop(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("stop river client: %w", err))
		}
	}

	s.pool.Close()
	return shutdownErr
}
