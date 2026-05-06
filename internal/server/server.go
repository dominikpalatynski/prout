package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/dominikpalatynski/prout/internal/event"
	"github.com/dominikpalatynski/prout/internal/github"
	"github.com/dominikpalatynski/prout/internal/log"
	"github.com/dominikpalatynski/prout/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	config             *config.Config
	http               *http.Server
	workspaceHandler   *workspace.WorkspaceHandler
	githubClient       *github.GithubClient
	githubEventHandler *event.GithubEventHandler
}

func NewServer(cfg *config.Config) (*Server, error) {
	if err := log.Init(); err != nil {
		return nil, err
	}

	githubClient, err := github.NewGithubClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}

	workspaceHandler, err := workspace.NewWorkspaceHandler(cfg, githubClient)
	if err != nil {
		return nil, fmt.Errorf("create workspace handler: %w", err)
	}

	githubEventHandler, err := event.NewGithubEventHandler(cfg, workspaceHandler, githubClient)
	if err != nil {
		return nil, fmt.Errorf("create github event handler: %w", err)
	}

	return &Server{
		config:             cfg,
		workspaceHandler:   workspaceHandler,
		githubEventHandler: githubEventHandler,
		githubClient:       githubClient,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting server", "port", s.config.Server.Port)
	r := chi.NewRouter()
	s.mount(r)
	s.http = &http.Server{
		Addr:    s.config.Server.Port,
		Handler: r,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", s.config.Server.Port)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutdown signal received")
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
		slog.ErrorContext(ctx, "Failed to shutdown server", "error", err)
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown http server: %w", err))
	}
	return shutdownErr
}

func (s *Server) mount(r chi.Router) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Post("/webhooks/github", s.handleGithubWebhook)

	r.Route("/settings", func(r chi.Router) {
		r.Get("/github-setup", s.githubSetupPageHandler)
		r.Post("/github-setup/start", s.githubSetupStartHandler)
		r.Get("/github-setup/callback", s.githubSetupCallbackHandler)
	})

	r.Route("/api", func(r chi.Router) {
		r.Get("/readyz", s.readyzHandler)
		r.Get("/healthz", s.healthz)
	})
}
