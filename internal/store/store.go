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
	App             string    `json:"app"`
	Ring            string    `json:"ring"`
	CurrentVersion  string    `json:"current_version"`
	PreviousVersion string    `json:"previous_version"`
	Healthy         bool      `json:"healthy"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// HistoryEntry is one recorded seed / promote / rollback event.
type HistoryEntry struct {
	ID          int64     `json:"id"`
	App         string    `json:"app"`
	Ring        string    `json:"ring"`
	Action      string    `json:"action"`
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
	Result      string    `json:"result"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store is the persistence interface. Implementations must be safe for
// concurrent use.
type Store interface {
	// GetRingState returns the state for one (app, ring). It returns
	// ErrNotFound if no state has been recorded yet.
	GetRingState(ctx context.Context, app, ring string) (RingState, error)
	// UpsertRingState creates or replaces the state for (state.App, state.Ring).
	UpsertRingState(ctx context.Context, state RingState) error
	// AddHistory appends an entry to the history log.
	AddHistory(ctx context.Context, entry HistoryEntry) error
	// ListHistory returns the history for an application, newest first.
	ListHistory(ctx context.Context, app string) ([]HistoryEntry, error)
	// Lock acquires an exclusive lock for key, blocking until it is held or ctx
	// is done. The returned function releases it. This serializes mutating
	// operations for one application. The Postgres implementation uses a session
	// advisory lock so the guarantee holds across multiple service replicas, not
	// just within one process.
	Lock(ctx context.Context, key string) (unlock func(), err error)
	// Close releases any underlying resources.
	Close() error
}
