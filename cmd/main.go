package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/server"
)

var version = "dev"

func main() {
	if err := root().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func root() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "toolshed",
		Short:   "Self-hosted GitHub preview environments",
		Version: version,
	}
	cmd.AddCommand(serverCmd())
	return cmd
}

func serverCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the HTTP server",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			srv, err := server.NewServer(cfg)
			if err != nil {
				return fmt.Errorf("init server: %w", err)
			}
			return srv.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", os.Getenv("TOOLSHED_CONFIG"), "path to server.yml")
	return cmd
}
