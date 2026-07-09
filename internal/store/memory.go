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

	lockMu sync.Mutex
	locks  map[string]*sync.Mutex
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		states: make(map[string]RingState),
		nextID: 1,
		now:    time.Now,
		locks:  make(map[string]*sync.Mutex),
	}
}

// Lock implements Store with a per-key in-process mutex. This is only correct
// within a single process; production multi-replica correctness comes from the
// Postgres implementation's advisory locks.
func (m *Memory) Lock(_ context.Context, key string) (func(), error) {
	m.lockMu.Lock()
	l, ok := m.locks[key]
	if !ok {
		l = &sync.Mutex{}
		m.locks[key] = l
	}
	m.lockMu.Unlock()

	l.Lock()
	return l.Unlock, nil
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

// UpsertRingState implements Store. The AutoPromote setting is preserved from
// any existing row — it changes only via SetAutoPromote.
func (m *Memory) UpsertRingState(_ context.Context, state RingState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state.UpdatedAt = m.now().UTC()
	if prev, ok := m.states[stateKey(state.App, state.Ring)]; ok {
		state.AutoPromote = prev.AutoPromote
	}
	m.states[stateKey(state.App, state.Ring)] = state
	return nil
}

// SetAutoPromote implements Store.
func (m *Memory) SetAutoPromote(_ context.Context, app, ring string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[stateKey(app, ring)]
	if !ok {
		s = RingState{App: app, Ring: ring, UpdatedAt: m.now().UTC()}
	}
	s.AutoPromote = enabled
	m.states[stateKey(app, ring)] = s
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
