package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func NewProvider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, files)
}

func Versions(ctx context.Context, db *sql.DB) (current, target int64, err error) {
	provider, err := NewProvider(db)
	if err != nil {
		return 0, 0, err
	}
	return provider.GetVersions(ctx)
}

func HasPending(ctx context.Context, db *sql.DB) (bool, error) {
	provider, err := NewProvider(db)
	if err != nil {
		return false, err
	}
	return provider.HasPending(ctx)
}

func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	provider, err := NewProvider(db)
	if err != nil {
		return nil, err
	}
	return provider.Up(ctx)
}

func Down(ctx context.Context, db *sql.DB) (*goose.MigrationResult, error) {
	provider, err := NewProvider(db)
	if err != nil {
		return nil, err
	}
	return provider.Down(ctx)
}

func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	provider, err := NewProvider(db)
	if err != nil {
		return nil, err
	}
	return provider.Status(ctx)
}
