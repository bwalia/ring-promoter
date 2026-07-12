package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
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
		INSERT INTO history (app, ring, action, from_version, to_version, result, message, logs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	if _, err := p.db.ExecContext(ctx, q, e.App, e.Ring, e.Action, e.FromVersion, e.ToVersion, e.Result, e.Message, e.Logs); err != nil {
		return fmt.Errorf("add history: %w", err)
	}
	if e.Logs == "" {
		return nil
	}
	// Keep detailed logs on only the newest KeepFailureLogs entries per app so
	// the table doesn't grow with every failure forever.
	const trim = `
		UPDATE history SET logs = '' WHERE app = $1 AND logs <> '' AND id NOT IN (
			SELECT id FROM history WHERE app = $1 AND logs <> '' ORDER BY id DESC LIMIT $2)`
	if _, err := p.db.ExecContext(ctx, trim, e.App, KeepFailureLogs); err != nil {
		return fmt.Errorf("trim failure logs: %w", err)
	}
	return nil
}

// ListHistory implements Store, newest first.
func (p *Postgres) ListHistory(ctx context.Context, app string) ([]HistoryEntry, error) {
	const q = `
		SELECT id, app, ring, action, from_version, to_version, result, message, diagnosis, created_at
		FROM history WHERE app = $1 ORDER BY id DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.App, &e.Ring, &e.Action, &e.FromVersion, &e.ToVersion, &e.Result, &e.Message, &e.Diagnosis, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetHistoryEntry implements Store (includes the stored failure logs).
func (p *Postgres) GetHistoryEntry(ctx context.Context, app string, id int64) (HistoryEntry, error) {
	const q = `
		SELECT id, app, ring, action, from_version, to_version, result, message, diagnosis, logs, created_at
		FROM history WHERE id = $1 AND app = $2`
	var e HistoryEntry
	err := p.db.QueryRowContext(ctx, q, id, app).Scan(
		&e.ID, &e.App, &e.Ring, &e.Action, &e.FromVersion, &e.ToVersion, &e.Result, &e.Message, &e.Diagnosis, &e.Logs, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryEntry{}, ErrNotFound
	}
	if err != nil {
		return HistoryEntry{}, fmt.Errorf("get history entry: %w", err)
	}
	return e, nil
}

// SetHistoryDiagnosis implements Store.
func (p *Postgres) SetHistoryDiagnosis(ctx context.Context, id int64, diagnosis string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE history SET diagnosis = $2 WHERE id = $1`, id, diagnosis)
	if err != nil {
		return fmt.Errorf("set history diagnosis: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListGroups implements Store, ordered by name.
func (p *Postgres) ListGroups(ctx context.Context) ([]Group, error) {
	const q = `SELECT id, name, apps, updated_at FROM app_group ORDER BY name, id`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		var apps string
		if err := rows.Scan(&g.ID, &g.Name, &apps, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		if err := json.Unmarshal([]byte(apps), &g.Apps); err != nil {
			return nil, fmt.Errorf("decode group %s apps: %w", g.ID, err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CreateGroup implements Store.
func (p *Postgres) CreateGroup(ctx context.Context, g Group) error {
	apps, err := json.Marshal(g.Apps)
	if err != nil {
		return fmt.Errorf("encode group apps: %w", err)
	}
	const q = `INSERT INTO app_group (id, name, apps) VALUES ($1, $2, $3)`
	if _, err := p.db.ExecContext(ctx, q, g.ID, g.Name, string(apps)); err != nil {
		return fmt.Errorf("create group: %w", err)
	}
	return nil
}

// UpdateGroup implements Store.
func (p *Postgres) UpdateGroup(ctx context.Context, g Group) error {
	apps, err := json.Marshal(g.Apps)
	if err != nil {
		return fmt.Errorf("encode group apps: %w", err)
	}
	const q = `UPDATE app_group SET name = $2, apps = $3, updated_at = now() WHERE id = $1`
	res, err := p.db.ExecContext(ctx, q, g.ID, g.Name, string(apps))
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteGroup implements Store.
func (p *Postgres) DeleteGroup(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM app_group WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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
