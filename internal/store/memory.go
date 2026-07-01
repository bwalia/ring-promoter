package store

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is an in-memory Store for local development and tests.
type Memory struct {
	mu      sync.RWMutex
	states  map[string]RingState // key: app + "\x00" + ring
	history []HistoryEntry
	nextID  int64
	now     func() time.Time
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		states: make(map[string]RingState),
		nextID: 1,
		now:    time.Now,
	}
}

func stateKey(app, ring string) string { return app + "\x00" + ring }

// GetRingState implements Store.
func (m *Memory) GetRingState(_ context.Context, app, ring string) (RingState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.states[stateKey(app, ring)]
	if !ok {
		return RingState{}, ErrNotFound
	}
	return s, nil
}

// UpsertRingState implements Store.
func (m *Memory) UpsertRingState(_ context.Context, state RingState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state.UpdatedAt = m.now().UTC()
	m.states[stateKey(state.App, state.Ring)] = state
	return nil
}

// AddHistory implements Store.
func (m *Memory) AddHistory(_ context.Context, entry HistoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.ID = m.nextID
	m.nextID++
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = m.now().UTC()
	}
	m.history = append(m.history, entry)
	return nil
}

// ListHistory implements Store, newest first.
func (m *Memory) ListHistory(_ context.Context, app string) ([]HistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []HistoryEntry
	for _, e := range m.history {
		if e.App == app {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Close implements Store.
func (m *Memory) Close() error { return nil }
