package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/server"
	"github.com/dominikpalatynski/toolshed/migrations"
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
	cmd.AddCommand(serverCmd(), migrateCmd())
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

			srv, err := server.New(cfg)
			if err != nil {
				return fmt.Errorf("init server: %w", err)
			}
			return srv.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", os.Getenv("TOOLSHED_CONFIG"), "path to server.yml")
	return cmd
}

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage database migrations",
	}
	cmd.AddCommand(
		migrateUpCmd(),
		migrateDownCmd(),
		migrateStatusCmd(),
		migrateVersionCmd(),
	)
	return cmd
}

func migrateUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply pending migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrationDB(cmd.Context(), func(db *sql.DB) error {
				results, err := migrations.Up(cmd.Context(), db)
				if err != nil {
					return fmt.Errorf("migrate up: %w", err)
				}
				if len(results) == 0 {
					fmt.Println("no pending migrations")
					return nil
				}
				for _, result := range results {
					fmt.Println(result.String())
				}
				return nil
			})
		},
	}
}

func migrateDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Roll back one migration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrationDB(cmd.Context(), func(db *sql.DB) error {
				result, err := migrations.Down(cmd.Context(), db)
				if err != nil {
					return fmt.Errorf("migrate down: %w", err)
				}
				if result == nil {
					fmt.Println("no migration rolled back")
					return nil
				}
				fmt.Println(result.String())
				return nil
			})
		},
	}
}

func migrateStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrationDB(cmd.Context(), func(db *sql.DB) error {
				statuses, err := migrations.Status(cmd.Context(), db)
				if err != nil {
					return fmt.Errorf("migrate status: %w", err)
				}
				fmt.Printf("%-9s %-24s %s\n", "STATE", "APPLIED AT", "MIGRATION")
				for _, status := range statuses {
					appliedAt := "-"
					if !status.AppliedAt.IsZero() {
						appliedAt = status.AppliedAt.UTC().Format(time.RFC3339)
					}
					fmt.Printf("%-9s %-24s %s\n", status.State, appliedAt, filepath.Base(status.Source.Path))
				}
				return nil
			})
		},
	}
}

func migrateVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show current and target migration versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMigrationDB(cmd.Context(), func(db *sql.DB) error {
				current, target, err := migrations.Versions(cmd.Context(), db)
				if err != nil {
					return fmt.Errorf("migrate version: %w", err)
				}
				fmt.Printf("current=%d target=%d\n", current, target)
				return nil
			})
		},
	}
}

func withMigrationDB(ctx context.Context, fn func(*sql.DB) error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	dsn := os.Getenv("TOOLSHED_DB_DSN")
	if dsn == "" {
		return fmt.Errorf("TOOLSHED_DB_DSN is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	return fn(db)
}
