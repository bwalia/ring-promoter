package bankapi

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"time"
)

// migrationFiles embeds the SQL migrations so the same binary can apply them on
// boot (RUN_MIGRATIONS=true) without shipping the .sql files separately. The
// Kubernetes migrate Job in the chart applies these same files with `psql`; the
// embedded copy keeps the two in lock-step.
//
//go:embed db/migrations/*.sql
var migrationFiles embed.FS

// Migrate is the entrypoint for the chart's migrate Job (the app image run with
// the "migrate" argument). Unlike the always-on server, this MUST fail loudly:
// it opens DATABASE_URL, waits for the database to become reachable, applies the
// embedded migrations, and returns an error on any failure so the Job is marked
// failed and the operator sees it. It is intentionally separate from Run so the
// server never dies on a database problem while the Job does.
func Migrate() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required for migrations")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Wait up to ~60s for the database (the Job may start before Postgres is
	// ready). Retry the ping rather than crash immediately.
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err = db.Ping(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("database not reachable within 60s: %w", err)
		}
		log.Printf("waiting for database... (%v)", err)
		time.Sleep(2 * time.Second)
	}

	log.Printf("applying embedded migrations")
	if err := runMigrations(db); err != nil {
		return err
	}
	log.Printf("migrations complete")
	return nil
}

// runMigrations applies every embedded migration in filename order against db.
// Migrations are written to be idempotent (IF NOT EXISTS / ON CONFLICT), so
// re-running them is safe. It returns the first error encountered.
func runMigrations(db *sql.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "db/migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := migrationFiles.ReadFile("db/migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}
