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
		INSERT INTO history (app, ring, action, from_version, to_version, result, message, logs, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if _, err := p.db.ExecContext(ctx, q, e.App, e.Ring, e.Action, e.FromVersion, e.ToVersion, e.Result, e.Message, e.Logs, e.CorrelationID); err != nil {
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
		SELECT id, app, ring, action, from_version, to_version, result, message, diagnosis, correlation_id, created_at
		FROM history WHERE app = $1 ORDER BY id DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.App, &e.Ring, &e.Action, &e.FromVersion, &e.ToVersion, &e.Result, &e.Message, &e.Diagnosis, &e.CorrelationID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetHistoryEntry implements Store (includes the stored failure logs).
func (p *Postgres) GetHistoryEntry(ctx context.Context, app string, id int64) (HistoryEntry, error) {
	const q = `
		SELECT id, app, ring, action, from_version, to_version, result, message, diagnosis, logs, correlation_id, created_at
		FROM history WHERE id = $1 AND app = $2`
	var e HistoryEntry
	err := p.db.QueryRowContext(ctx, q, id, app).Scan(
		&e.ID, &e.App, &e.Ring, &e.Action, &e.FromVersion, &e.ToVersion, &e.Result, &e.Message, &e.Diagnosis, &e.Logs, &e.CorrelationID, &e.CreatedAt)
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

// ListTopologyEdges implements Store.
func (p *Postgres) ListTopologyEdges(ctx context.Context) ([]TopologyEdge, error) {
	return p.listTopologyEdges(ctx, `SELECT from_app, to_app FROM topology_edge ORDER BY from_app, to_app`)
}

// AddTopologyEdge implements Store.
func (p *Postgres) AddTopologyEdge(ctx context.Context, from, to string) error {
	const q = `INSERT INTO topology_edge (from_app, to_app) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if _, err := p.db.ExecContext(ctx, q, from, to); err != nil {
		return fmt.Errorf("add topology edge: %w", err)
	}
	return nil
}

// DeleteTopologyEdge implements Store.
func (p *Postgres) DeleteTopologyEdge(ctx context.Context, from, to string) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM topology_edge WHERE from_app = $1 AND to_app = $2`, from, to); err != nil {
		return fmt.Errorf("delete topology edge: %w", err)
	}
	return nil
}

// ListTopologySuppressions implements Store.
func (p *Postgres) ListTopologySuppressions(ctx context.Context) ([]TopologyEdge, error) {
	return p.listTopologyEdges(ctx, `SELECT from_app, to_app FROM topology_suppression ORDER BY from_app, to_app`)
}

// AddTopologySuppression implements Store.
func (p *Postgres) AddTopologySuppression(ctx context.Context, from, to string) error {
	const q = `INSERT INTO topology_suppression (from_app, to_app) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if _, err := p.db.ExecContext(ctx, q, from, to); err != nil {
		return fmt.Errorf("add topology suppression: %w", err)
	}
	return nil
}

// DeleteTopologySuppression implements Store.
func (p *Postgres) DeleteTopologySuppression(ctx context.Context, from, to string) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM topology_suppression WHERE from_app = $1 AND to_app = $2`, from, to); err != nil {
		return fmt.Errorf("delete topology suppression: %w", err)
	}
	return nil
}

func (p *Postgres) listTopologyEdges(ctx context.Context, query string) ([]TopologyEdge, error) {
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list topology edges: %w", err)
	}
	defer rows.Close()
	var out []TopologyEdge
	for rows.Next() {
		var edge TopologyEdge
		if err := rows.Scan(&edge.From, &edge.To); err != nil {
			return nil, fmt.Errorf("scan topology edge: %w", err)
		}
		out = append(out, edge)
	}
	return out, rows.Err()
}

// CreateMaintenanceWindow implements Store. It prunes windows that ended more
// than pruneWindowAfter ago in the same statement batch.
func (p *Postgres) CreateMaintenanceWindow(ctx context.Context, w MaintenanceWindow) error {
	const q = `
		INSERT INTO maintenance_window (id, app, ring, starts_at, ends_at, reason, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := p.db.ExecContext(ctx, q, w.ID, w.App, w.Ring, w.StartsAt, w.EndsAt, w.Reason, w.CreatedBy); err != nil {
		return fmt.Errorf("create maintenance window: %w", err)
	}
	const prune = `DELETE FROM maintenance_window WHERE ends_at < now() - ($1::bigint * interval '1 second')`
	if _, err := p.db.ExecContext(ctx, prune, int64(pruneWindowAfter.Seconds())); err != nil {
		return fmt.Errorf("prune maintenance windows: %w", err)
	}
	return nil
}

// ListMaintenanceWindows implements Store, newest first.
func (p *Postgres) ListMaintenanceWindows(ctx context.Context, app string) ([]MaintenanceWindow, error) {
	const q = `
		SELECT id, app, ring, starts_at, ends_at, reason, created_by, created_at
		FROM maintenance_window WHERE app = $1 ORDER BY starts_at DESC, id DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list maintenance windows: %w", err)
	}
	defer rows.Close()
	var out []MaintenanceWindow
	for rows.Next() {
		var w MaintenanceWindow
		if err := rows.Scan(&w.ID, &w.App, &w.Ring, &w.StartsAt, &w.EndsAt, &w.Reason, &w.CreatedBy, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan maintenance window: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteMaintenanceWindow implements Store (scoped to the owning app).
func (p *Postgres) DeleteMaintenanceWindow(ctx context.Context, app, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM maintenance_window WHERE id = $1 AND app = $2`, id, app)
	if err != nil {
		return fmt.Errorf("delete maintenance window: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertSignoff implements Store.
func (p *Postgres) UpsertSignoff(ctx context.Context, s Signoff) error {
	const q = `
		INSERT INTO signoff (app, ring, version, decision, engineer, qa_status, note, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (app, ring, version) DO UPDATE SET
			decision   = EXCLUDED.decision,
			engineer   = EXCLUDED.engineer,
			qa_status  = EXCLUDED.qa_status,
			note       = EXCLUDED.note,
			updated_at = now()`
	if _, err := p.db.ExecContext(ctx, q, s.App, s.Ring, s.Version, s.Decision, s.Engineer, s.QAStatus, s.Note); err != nil {
		return fmt.Errorf("upsert signoff: %w", err)
	}
	return nil
}

// GetSignoff implements Store.
func (p *Postgres) GetSignoff(ctx context.Context, app, ring, version string) (Signoff, error) {
	const q = `
		SELECT app, ring, version, decision, engineer, qa_status, note, updated_at
		FROM signoff WHERE app = $1 AND ring = $2 AND version = $3`
	var s Signoff
	err := p.db.QueryRowContext(ctx, q, app, ring, version).Scan(
		&s.App, &s.Ring, &s.Version, &s.Decision, &s.Engineer, &s.QAStatus, &s.Note, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Signoff{}, ErrNotFound
	}
	if err != nil {
		return Signoff{}, fmt.Errorf("get signoff: %w", err)
	}
	return s, nil
}

// ListSignoffs implements Store, newest first.
func (p *Postgres) ListSignoffs(ctx context.Context, app string) ([]Signoff, error) {
	const q = `
		SELECT app, ring, version, decision, engineer, qa_status, note, updated_at
		FROM signoff WHERE app = $1 ORDER BY updated_at DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list signoffs: %w", err)
	}
	defer rows.Close()
	var out []Signoff
	for rows.Next() {
		var s Signoff
		if err := rows.Scan(&s.App, &s.Ring, &s.Version, &s.Decision, &s.Engineer, &s.QAStatus, &s.Note, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan signoff: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreatePendingOp implements Store.
func (p *Postgres) CreatePendingOp(ctx context.Context, op PendingOp) (int64, error) {
	const q = `
		INSERT INTO pending_op (app, ring, action, from_ring, version, prev_version, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	var id int64
	if err := p.db.QueryRowContext(ctx, q, op.App, op.Ring, op.Action, op.FromRing, op.Version, op.PrevVersion, op.CorrelationID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create pending op: %w", err)
	}
	return id, nil
}

// GetPendingOp implements Store.
func (p *Postgres) GetPendingOp(ctx context.Context, id int64) (PendingOp, error) {
	const q = `
		SELECT id, app, ring, action, from_ring, version, prev_version, correlation_id, started_at
		FROM pending_op WHERE id = $1`
	var op PendingOp
	err := p.db.QueryRowContext(ctx, q, id).Scan(
		&op.ID, &op.App, &op.Ring, &op.Action, &op.FromRing, &op.Version, &op.PrevVersion, &op.CorrelationID, &op.StartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingOp{}, ErrNotFound
	}
	if err != nil {
		return PendingOp{}, fmt.Errorf("get pending op: %w", err)
	}
	return op, nil
}

// DeletePendingOp implements Store (idempotent).
func (p *Postgres) DeletePendingOp(ctx context.Context, id int64) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM pending_op WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete pending op: %w", err)
	}
	return nil
}

// ListPendingOps implements Store, oldest first.
func (p *Postgres) ListPendingOps(ctx context.Context) ([]PendingOp, error) {
	const q = `
		SELECT id, app, ring, action, from_ring, version, prev_version, correlation_id, started_at
		FROM pending_op ORDER BY id`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list pending ops: %w", err)
	}
	defer rows.Close()
	var out []PendingOp
	for rows.Next() {
		var op PendingOp
		if err := rows.Scan(&op.ID, &op.App, &op.Ring, &op.Action, &op.FromRing, &op.Version, &op.PrevVersion, &op.CorrelationID, &op.StartedAt); err != nil {
			return nil, fmt.Errorf("scan pending op: %w", err)
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

// AppendAudit implements Store. The ledger is append-only by construction:
// this is the only statement that touches audit_event besides ListAudit.
func (p *Postgres) AppendAudit(ctx context.Context, e AuditEvent) error {
	const q = `
		INSERT INTO audit_event (correlation_id, actor_type, actor, app, ring, category, action, version, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	detail := e.Detail
	if detail == "" {
		detail = "{}"
	}
	if _, err := p.db.ExecContext(ctx, q,
		e.CorrelationID, e.ActorType, e.Actor, e.App, e.Ring, e.Category, e.Action, e.Version, detail); err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// ListAudit implements Store, newest first with keyset paging.
func (p *Postgres) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEvent, error) {
	q := `
		SELECT id, occurred_at, correlation_id, actor_type, actor, app, ring, category, action, version, detail
		FROM audit_event WHERE 1=1`
	var args []any
	add := func(clause, val string) {
		if val != "" {
			args = append(args, val)
			q += fmt.Sprintf(" AND %s = $%d", clause, len(args))
		}
	}
	add("app", f.App)
	add("ring", f.Ring)
	add("category", f.Category)
	add("actor", f.Actor)
	if f.BeforeID > 0 {
		args = append(args, f.BeforeID)
		q += fmt.Sprintf(" AND id < $%d", len(args))
	}
	args = append(args, clampAuditLimit(f.Limit))
	q += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args))

	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.CorrelationID, &e.ActorType, &e.Actor,
			&e.App, &e.Ring, &e.Category, &e.Action, &e.Version, &e.Detail); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
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
