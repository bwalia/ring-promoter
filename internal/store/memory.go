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
	groups  map[string]Group
	nextID  int64
	now     func() time.Time

	lockMu sync.Mutex
	locks  map[string]*sync.Mutex
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		states: make(map[string]RingState),
		groups: make(map[string]Group),
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
	if entry.Logs != "" {
		m.trimFailureLogsLocked(entry.App)
	}
	return nil
}

// trimFailureLogsLocked keeps detailed logs on only the newest KeepFailureLogs
// entries of an app, clearing older ones. Callers must hold m.mu.
func (m *Memory) trimFailureLogsLocked(app string) {
	kept := 0
	for i := len(m.history) - 1; i >= 0; i-- {
		e := &m.history[i]
		if e.App != app || e.Logs == "" {
			continue
		}
		kept++
		if kept > KeepFailureLogs {
			e.Logs = ""
		}
	}
}

// ListHistory implements Store, newest first. Logs are omitted.
func (m *Memory) ListHistory(_ context.Context, app string) ([]HistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []HistoryEntry
	for _, e := range m.history {
		if e.App == app {
			e.Logs = ""
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// GetHistoryEntry implements Store.
func (m *Memory) GetHistoryEntry(_ context.Context, app string, id int64) (HistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.history {
		if e.ID == id && e.App == app {
			return e, nil
		}
	}
	return HistoryEntry{}, ErrNotFound
}

// SetHistoryDiagnosis implements Store.
func (m *Memory) SetHistoryDiagnosis(_ context.Context, id int64, diagnosis string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.history {
		if m.history[i].ID == id {
			m.history[i].Diagnosis = diagnosis
			return nil
		}
	}
	return ErrNotFound
}

// ListGroups implements Store, ordered by name (then ID for stability).
func (m *Memory) ListGroups(_ context.Context) ([]Group, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Group, 0, len(m.groups))
	for _, g := range m.groups {
		g.Apps = append([]string(nil), g.Apps...)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// CreateGroup implements Store.
func (m *Memory) CreateGroup(_ context.Context, g Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g.Apps = append([]string(nil), g.Apps...)
	g.UpdatedAt = m.now().UTC()
	m.groups[g.ID] = g
	return nil
}

// UpdateGroup implements Store.
func (m *Memory) UpdateGroup(_ context.Context, g Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[g.ID]; !ok {
		return ErrNotFound
	}
	g.Apps = append([]string(nil), g.Apps...)
	g.UpdatedAt = m.now().UTC()
	m.groups[g.ID] = g
	return nil
}

// DeleteGroup implements Store.
func (m *Memory) DeleteGroup(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[id]; !ok {
		return ErrNotFound
	}
	delete(m.groups, id)
	return nil
}

// Close implements Store.
func (m *Memory) Close() error { return nil }
