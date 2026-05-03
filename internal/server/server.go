package server

import (
	"context"
	"log/slog"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/log"
)

type Server struct {
	config *config.Config
}

func NewServer(cfg *config.Config) (*Server, error) {
	if err := log.Init(); err != nil {
		return nil, err
	}

	return &Server{config: cfg}, nil
}

func (s *Server) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting server", "port", s.config.Server.Port)
	return nil
}
