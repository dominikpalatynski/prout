package testdb

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/dominikpalatynski/toolshed/migrations"
)

var configureDockerEnvOnce sync.Once

// Start spins up a Postgres testcontainer scoped to the test, returns a connected pool.
// Migrations are applied via goose; callers may skip with -short to bypass.
func Start(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration: skipped in -short mode")
	}

	configureDockerAccessForColima()

	ctx := context.Background()
	pgC, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("toolshed"),
		postgres.WithUsername("toolshed"),
		postgres.WithPassword("toolshed"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := migrations.Up(ctx, sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return pool
}

func configureDockerAccessForColima() {
	configureDockerEnvOnce.Do(func() {
		if _, exists := os.LookupEnv("DOCKER_HOST"); exists {
			return
		}

		dockerHost, ok := defaultColimaDockerHost()
		if !ok {
			return
		}

		_ = os.Setenv("DOCKER_HOST", dockerHost)

		if _, exists := os.LookupEnv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE"); !exists {
			// Colima exposes the Docker API over a user-space socket on macOS, but the
			// reaper container must still mount the in-VM Docker socket path.
			_ = os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
		}
	})
}

func defaultColimaDockerHost() (string, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	return defaultColimaDockerHostForHome(homeDir)
}

func defaultColimaDockerHostForHome(homeDir string) (string, bool) {
	socketPath := filepath.Join(homeDir, ".colima", "default", "docker.sock")
	if _, err := os.Stat(socketPath); err != nil {
		return "", false
	}

	return "unix://" + socketPath, true
}
