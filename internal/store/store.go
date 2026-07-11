// Package store persists per-(application, ring) state and an append-only
// promotion history. Two implementations are provided: an in-memory store for
// local development / tests and a Postgres store for production.
package store

import (
	"context"
	"errors"
	"time"
)

// Action names recorded in history.
const (
	ActionSeed     = "seed"
	ActionPromote  = "promote"
	ActionRollback = "rollback"
)

// Result values recorded in history.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// ErrNotFound is returned when a ring state does not yet exist.
var ErrNotFound = errors.New("ring state not found")

// RingState is the tracked state of one application in one ring.
type RingState struct {
	App             string `json:"app"`
	Ring            string `json:"ring"`
	CurrentVersion  string `json:"current_version"`
	PreviousVersion string `json:"previous_version"`
	Healthy         bool   `json:"healthy"`
	// AutoPromote: when a version lands healthy in this ring, it is promoted
	// onward to the next ring automatically. This is a setting, not deploy
	// state: it is changed ONLY via SetAutoPromote — UpsertRingState leaves it
	// untouched.
	AutoPromote bool      `json:"auto_promote"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Group is a user-defined collection of applications, shared by every user of
// this control plane (persisted server-side, not in the browser).
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Apps      []string  `json:"apps"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HistoryEntry is one recorded seed / promote / rollback event.
type HistoryEntry struct {
	ID          int64  `json:"id"`
	App         string `json:"app"`
	Ring        string `json:"ring"`
	Action      string `json:"action"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Result      string `json:"result"`
	Message     string `json:"message"`
	// Diagnosis is the stored AI explanation of a failed entry (empty until
	// someone asks for one). Persisted so every user sees the same answer and
	// it survives restarts.
	Diagnosis string `json:"diagnosis,omitempty"`
	// Logs holds the step-by-step logs captured when this entry failed, so AI
	// diagnosis has real evidence even after the in-memory job is gone. Kept
	// only for the newest KeepFailureLogs failures per app (older entries are
	// trimmed back to ""); never serialized to API clients.
	Logs      string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// KeepFailureLogs is how many recent failures per application retain their
// detailed step logs (older ones keep only the summary fields).
const KeepFailureLogs = 3

// Store is the persistence interface. Implementations must be safe for
// concurrent use.
type Store interface {
	// GetRingState returns the state for one (app, ring). It returns
	// ErrNotFound if no state has been recorded yet.
	GetRingState(ctx context.Context, app, ring string) (RingState, error)
	// UpsertRingState creates or replaces the deploy state for
	// (state.App, state.Ring). It never modifies the AutoPromote setting.
	UpsertRingState(ctx context.Context, state RingState) error
	// SetAutoPromote flips the auto-promote setting for (app, ring), creating
	// the row (with empty versions) if none exists yet.
	SetAutoPromote(ctx context.Context, app, ring string, enabled bool) error
	// AddHistory appends an entry to the history log. Storing an entry with
	// Logs also trims logs of older entries beyond the newest KeepFailureLogs
	// for that app.
	AddHistory(ctx context.Context, entry HistoryEntry) error
	// ListHistory returns the history for an application, newest first. Logs
	// are omitted (potentially large) — use GetHistoryEntry for them.
	ListHistory(ctx context.Context, app string) ([]HistoryEntry, error)
	// GetHistoryEntry returns one history entry of an application, including
	// its Logs. It returns ErrNotFound when no such entry exists (or it
	// belongs to another app).
	GetHistoryEntry(ctx context.Context, app string, id int64) (HistoryEntry, error)
	// SetHistoryDiagnosis stores the AI diagnosis for a history entry. It
	// returns ErrNotFound when the entry does not exist.
	SetHistoryDiagnosis(ctx context.Context, id int64, diagnosis string) error
	// ListGroups returns all application groups, ordered by name.
	ListGroups(ctx context.Context) ([]Group, error)
	// CreateGroup stores a new group (the caller assigns a unique ID).
	CreateGroup(ctx context.Context, g Group) error
	// UpdateGroup replaces an existing group's name and members. It returns
	// ErrNotFound when no group with g.ID exists.
	UpdateGroup(ctx context.Context, g Group) error
	// DeleteGroup removes a group. It returns ErrNotFound when absent.
	DeleteGroup(ctx context.Context, id string) error
	// Lock acquires an exclusive lock for key, blocking until it is held or ctx
	// is done. The returned function releases it. This serializes mutating
	// operations for one application. The Postgres implementation uses a session
	// advisory lock so the guarantee holds across multiple service replicas, not
	// just within one process.
	Lock(ctx context.Context, key string) (unlock func(), err error)
	// Close releases any underlying resources.
	Close() error
}
