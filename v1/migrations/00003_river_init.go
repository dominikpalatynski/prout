package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

func init() {
	goose.AddNamedMigrationNoTxContext("00003_river_init.go", upRiverInit, downRiverInit)
}

func upRiverInit(ctx context.Context, db *sql.DB) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(db), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func downRiverInit(ctx context.Context, db *sql.DB) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(db), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{
		TargetVersion: -1,
	})
	return err
}
