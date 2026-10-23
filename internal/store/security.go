package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// MaxStoredFindings caps the findings kept per scan; the severity counts
// always reflect the full report.
const MaxStoredFindings = 500

// keepScansPerApp bounds the in-memory store's scans per application.
const keepScansPerApp = 500

// SecurityFinding is one vulnerability or weakness a scanner reported.
type SecurityFinding struct {
	// ID is the rule or advisory id (e.g. CVE-2024-1234, a Semgrep rule id);
	// waivers match it exactly.
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title,omitempty"`
	Location string `json:"location,omitempty"`
	// Package, Installed and Fixed describe a vulnerable dependency.
	Package   string `json:"package,omitempty"`
	Installed string `json:"installed,omitempty"`
	Fixed     string `json:"fixed,omitempty"`
	// Waived is set on read when an active waiver matches the finding.
	Waived bool `json:"waived,omitempty"`
}

// SecurityScan is the latest report one scanner made for one kind of scan of
// one exact (app, version) — the evidence a security gate reads.
type SecurityScan struct {
	ID       string            `json:"id"`
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Kind     string            `json:"kind"`
	Scanner  string            `json:"scanner"`
	Critical int               `json:"critical"`
	High     int               `json:"high"`
	Medium   int               `json:"medium"`
	Low      int               `json:"low"`
	Findings []SecurityFinding `json:"findings"`
	Summary  string            `json:"summary,omitempty"`
	// URL links to the scan's run or report.
	URL       string    `json:"url,omitempty"`
	ScannedAt time.Time `json:"scanned_at"`
	CreatedAt time.Time `json:"created_at"`
}

// SecurityWaiver is a time-boxed, attributed risk acceptance for one finding
// id of one app (on one ring, or every ring when Ring is empty).
type SecurityWaiver struct {
	ID        string    `json:"id"`
	App       string    `json:"app"`
	Finding   string    `json:"finding"`
	Ring      string    `json:"ring"`
	Reason    string    `json:"reason"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ActiveAt reports whether the waiver is in force at t.
func (w SecurityWaiver) ActiveAt(t time.Time) bool { return t.Before(w.ExpiresAt) }

// Covers reports whether the waiver applies to findingID on ringName ("" ring
// means "any ring": an evaluation not tied to a ring is covered only by
// ring-wide waivers).
func (w SecurityWaiver) Covers(findingID, ringName string) bool {
	return w.Finding == findingID && (w.Ring == "" || w.Ring == ringName)
}

func securityScanKey(app, version, kind, scanner string) string {
	return app + "\x00" + version + "\x00" + kind + "\x00" + scanner
}

// UpsertSecurityScan implements Store.
func (m *Memory) UpsertSecurityScan(_ context.Context, s SecurityScan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := securityScanKey(s.App, s.Version, s.Kind, s.Scanner)
	s.CreatedAt = m.now().UTC()
	if s.ScannedAt.IsZero() {
		s.ScannedAt = s.CreatedAt
	}
	s.Findings = append([]SecurityFinding{}, s.Findings...)
	m.securityScans[key] = s
	// Prune the app's oldest scans beyond the cap.
	var keys []string
	for k, v := range m.securityScans {
		if v.App == s.App {
			keys = append(keys, k)
		}
	}
	if len(keys) > keepScansPerApp {
		sort.Slice(keys, func(i, j int) bool {
			return m.securityScans[keys[i]].CreatedAt.After(m.securityScans[keys[j]].CreatedAt)
		})
		for _, k := range keys[keepScansPerApp:] {
			delete(m.securityScans, k)
		}
	}
	return nil
}

// ListSecurityScans implements Store, newest first.
func (m *Memory) ListSecurityScans(_ context.Context, app, version string) ([]SecurityScan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []SecurityScan
	for _, s := range m.securityScans {
		if s.App == app && (version == "" || s.Version == version) {
			s.Findings = append([]SecurityFinding{}, s.Findings...)
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// CreateSecurityWaiver implements Store.
func (m *Memory) CreateSecurityWaiver(_ context.Context, w SecurityWaiver) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = m.now().UTC()
	}
	m.securityWaivers[w.ID] = w
	return nil
}

// ListSecurityWaivers implements Store, newest first.
func (m *Memory) ListSecurityWaivers(_ context.Context, app string) ([]SecurityWaiver, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []SecurityWaiver
	for _, w := range m.securityWaivers {
		if w.App == app {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// DeleteSecurityWaiver implements Store (scoped to the owning app).
func (m *Memory) DeleteSecurityWaiver(_ context.Context, app, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.securityWaivers[id]
	if !ok || w.App != app {
		return ErrNotFound
	}
	delete(m.securityWaivers, id)
	return nil
}

// UpsertSecurityScan implements Store.
func (p *Postgres) UpsertSecurityScan(ctx context.Context, s SecurityScan) error {
	findings, err := json.Marshal(s.Findings)
	if err != nil {
		return fmt.Errorf("encode findings: %w", err)
	}
	scannedAt := s.ScannedAt
	if scannedAt.IsZero() {
		scannedAt = time.Now().UTC()
	}
	const q = `
		INSERT INTO security_scan (id, app, version, kind, scanner, critical, high, medium, low, findings, summary, url, scanned_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
		ON CONFLICT (app, version, kind, scanner) DO UPDATE SET
			id = EXCLUDED.id, critical = EXCLUDED.critical, high = EXCLUDED.high,
			medium = EXCLUDED.medium, low = EXCLUDED.low, findings = EXCLUDED.findings,
			summary = EXCLUDED.summary, url = EXCLUDED.url,
			scanned_at = EXCLUDED.scanned_at, created_at = now()`
	if _, err := p.db.ExecContext(ctx, q, s.ID, s.App, s.Version, s.Kind, s.Scanner,
		s.Critical, s.High, s.Medium, s.Low, string(findings), s.Summary, s.URL, scannedAt); err != nil {
		return fmt.Errorf("upsert security scan: %w", err)
	}
	return nil
}

// ListSecurityScans implements Store, newest first.
func (p *Postgres) ListSecurityScans(ctx context.Context, app, version string) ([]SecurityScan, error) {
	const cols = `id, app, version, kind, scanner, critical, high, medium, low, findings, summary, url, scanned_at, created_at`
	var (
		rows *sql.Rows
		err  error
	)
	if version == "" {
		rows, err = p.db.QueryContext(ctx, `SELECT `+cols+` FROM security_scan WHERE app = $1 ORDER BY created_at DESC, id DESC`, app)
	} else {
		rows, err = p.db.QueryContext(ctx, `SELECT `+cols+` FROM security_scan WHERE app = $1 AND version = $2 ORDER BY created_at DESC, id DESC`, app, version)
	}
	if err != nil {
		return nil, fmt.Errorf("list security scans: %w", err)
	}
	defer rows.Close()
	var out []SecurityScan
	for rows.Next() {
		var (
			s        SecurityScan
			findings string
		)
		if err := rows.Scan(&s.ID, &s.App, &s.Version, &s.Kind, &s.Scanner, &s.Critical, &s.High, &s.Medium, &s.Low,
			&findings, &s.Summary, &s.URL, &s.ScannedAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan security scan: %w", err)
		}
		if findings != "" {
			if err := json.Unmarshal([]byte(findings), &s.Findings); err != nil {
				return nil, fmt.Errorf("decode findings of %s: %w", s.ID, err)
			}
		}
		if s.Findings == nil {
			s.Findings = []SecurityFinding{}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateSecurityWaiver implements Store.
func (p *Postgres) CreateSecurityWaiver(ctx context.Context, w SecurityWaiver) error {
	const q = `
		INSERT INTO security_waiver (id, app, finding, ring, reason, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := p.db.ExecContext(ctx, q, w.ID, w.App, w.Finding, w.Ring, w.Reason, w.CreatedBy, w.ExpiresAt); err != nil {
		return fmt.Errorf("create security waiver: %w", err)
	}
	return nil
}

// ListSecurityWaivers implements Store, newest first.
func (p *Postgres) ListSecurityWaivers(ctx context.Context, app string) ([]SecurityWaiver, error) {
	const q = `
		SELECT id, app, finding, ring, reason, created_by, created_at, expires_at
		FROM security_waiver WHERE app = $1 ORDER BY created_at DESC, id DESC`
	rows, err := p.db.QueryContext(ctx, q, app)
	if err != nil {
		return nil, fmt.Errorf("list security waivers: %w", err)
	}
	defer rows.Close()
	var out []SecurityWaiver
	for rows.Next() {
		var w SecurityWaiver
		if err := rows.Scan(&w.ID, &w.App, &w.Finding, &w.Ring, &w.Reason, &w.CreatedBy, &w.CreatedAt, &w.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan security waiver: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteSecurityWaiver implements Store (scoped to the owning app).
func (p *Postgres) DeleteSecurityWaiver(ctx context.Context, app, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM security_waiver WHERE id = $1 AND app = $2`, id, app)
	if err != nil {
		return fmt.Errorf("delete security waiver: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
