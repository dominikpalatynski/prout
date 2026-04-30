package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

func init() {
	goose.AddMigrationContext(upRiverInit, downRiverInit)
}

func upRiverInit(ctx context.Context, tx *sql.Tx) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(nil), nil)
	if err != nil {
		return err
	}
	_, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionUp, nil)
	return err
}

func downRiverInit(ctx context.Context, tx *sql.Tx) error {
	migrator, err := rivermigrate.New(riverdatabasesql.New(nil), nil)
	if err != nil {
		return err
	}
	_, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionDown, nil)
	return err
}
