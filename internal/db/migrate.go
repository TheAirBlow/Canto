package db

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

// migrationsFS holds the embedded goose migration files.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every pending migration to sqlDB.
func Migrate(sqlDB *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(sqlDB, "migrations")
}
