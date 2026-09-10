package repository

import (
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func (r *PostgresMediaRepository) Migrate() error {
	files, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	driver, err := pgxmigrate.WithInstance(r.conn.DB, &pgxmigrate.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", files, "pgx5", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
