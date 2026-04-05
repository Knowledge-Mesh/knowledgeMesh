package control

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	postgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // register postgres driver for database/sql (migrations only)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies embedded versioned SQL migrations (golang-migrate).
// Call with the same DATABASE_URL used by the app pool. Safe on every startup; applied versions are skipped.
func RunMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("migrate sql open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("migrate ping: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate database driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate embedded source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return err
	}
	return nil
}
