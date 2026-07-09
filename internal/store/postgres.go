package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schemaSQL string

// Postgres is a Store backed by PostgreSQL.
type Postgres struct {
	db *sql.DB
}

// NewPostgres opens a connection pool, verifies it, and applies the schema.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	p := &Postgres{db: db}
	if err := p.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) migrate(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// GetRingState implements Store.
func (p *Postgres) GetRingState(ctx context.Context, app, ring string) (RingState, error) {
	const q = `
		SELECT app, ring, current_version, previous_version, healthy, auto_promote, updated_at
		FROM ring_state WHERE app = $1 AND ring = $2`
	var s RingState
	err := p.db.QueryRowContext(ctx, q, app, ring).Scan(
		&s.App, &s.Ring, &s.CurrentVersion, &s.PreviousVersion, &s.Healthy, &s.AutoPromote, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RingState{}, ErrNotFound
	}
	if err != nil {
		return RingState{}, fmt.Errorf("get ring state: %w", err)
	}
	return s, nil
}

// UpsertRingState implements Store. The auto_promote column is deliberately
// not touched — it is a setting, changed only via SetAutoPromote.
func (p *Postgres) UpsertRingState(ctx context.Context, s RingState) error {
	const q = `
		INSERT INTO ring_state (app, ring, current_version, previous_version, healthy, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (app, ring) DO UPDATE SET
			current_version  = EXCLUDED.current_version,
			previous_version = EXCLUDED.previous_version,
			healthy          = EXCLUDED.healthy,
			updated_at       = now()`
	if _, err := p.db.ExecContext(ctx, q, s.App, s.Ring, s.CurrentVersion, s.PreviousVersion, s.Healthy); err != nil {
		return fmt.Errorf("upsert ring state: %w", err)
	}
	return nil
}

// SetAutoPromote implements Store.
func (p *Postgres) SetAutoPromote(ctx context.Context, app, ring string, enabled bool) error {
	const q = `
		INSERT INTO ring_state (app, ring, auto_promote)
		VALUES ($1, $2, $3)
		ON CONFLICT (app, ring) DO UPDATE SET auto_promote = EXCLUDED.auto_promote`
	if _, err := p.db.ExecContext(ctx, q, app, ring, enabled); err != nil {
		return fmt.Errorf("set auto promote: %w", err)
	}
	return nil
}

// AddHistory implements Store.
func (p *Postgres) AddHistory(ctx context.Context, e HistoryEntry) error {
	const q = `
		INSERT INTO history (app, ring, action, from_version, to_version, result, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := p.db.ExecContext(ctx, q, e.App, e.Ring, e.Action, e.FromVersion, e.ToVersion, e.Result, e.Message); err != nil {
		return fmt.Errorf("add history: %w", err)
	}
	return nil
}

// ListHistory implements Store, newest first.
func (p *Postgres) ListHistory(ctx context.Context, app string) ([]HistoryEntry, error) {
	const q = `
		SELECT id, app, ring, action, from_version, to_version, result, message, created_at
		FROM history WHERE app = $1 ORDER BY id DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.App, &e.Ring, &e.Action, &e.FromVersion, &e.ToVersion, &e.Result, &e.Message, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Lock implements Store using a PostgreSQL session-level advisory lock, held on
// a dedicated connection. This serializes operations for a key across ALL
// service replicas — not just within one process — so an accidental scale-up
// cannot run two concurrent promotions on the same application. If the process
// dies, the session ends and Postgres releases the lock automatically.
func (p *Postgres) Lock(ctx context.Context, key string) (func(), error) {
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	// hashtextextended maps the namespaced key to the bigint pg_advisory_lock wants.
	const ns = "ringpromoter:"
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", ns+key); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire advisory lock: %w", err)
	}
	return func() {
		// Best effort: explicitly unlock, then close. Closing the connection ends
		// the session, which releases the advisory lock regardless. Use a fresh
		// context so shutdown cancellation cannot strand the lock.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", ns+key)
		_ = conn.Close()
	}, nil
}

// Close implements Store.
func (p *Postgres) Close() error { return p.db.Close() }
