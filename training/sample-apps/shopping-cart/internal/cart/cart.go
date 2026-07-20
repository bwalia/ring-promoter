// Package cart is the storage layer for the shopping-cart backend. It hides
// whether items live in Redis or in process memory behind a single Store
// interface, so the HTTP layer never has to care — and never crashes when Redis
// is unavailable.
package cart

import (
	"sync"

	"github.com/bwalia/ring-promoter/training/shopping-cart/internal/redisclient"
)

// listKey is the Redis list holding cart items.
const listKey = "shopping-cart:items"

// Store is the minimal contract the API needs.
type Store interface {
	// List returns the current cart items (newest first). It must return an
	// empty slice — not an error — when the backing store is merely empty.
	List() ([]string, error)
	// Add appends an item to the cart.
	Add(item string) error
	// Ping reports whether the backing store is reachable.
	Ping() error
	// Backend names the implementation for diagnostics ("redis" or "memory").
	Backend() string
}

// MemoryStore is an in-process fallback used when REDIS_ADDR is unset. It keeps
// the app fully usable for local development and demos.
type MemoryStore struct {
	mu    sync.Mutex
	items []string
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (m *MemoryStore) List() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.items))
	copy(out, m.items)
	return out, nil
}

func (m *MemoryStore) Add(item string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Prepend so newest is first, matching Redis LPUSH + LRANGE semantics.
	m.items = append([]string{item}, m.items...)
	return nil
}

func (m *MemoryStore) Ping() error     { return nil }
func (m *MemoryStore) Backend() string { return "memory" }

// RedisStore persists items in Redis using LPUSH / LRANGE.
type RedisStore struct {
	c *redisclient.Client
}

// NewRedisStore wraps a Redis client.
func NewRedisStore(c *redisclient.Client) *RedisStore { return &RedisStore{c: c} }

// List returns items, or an empty slice if the list does not exist. A transport
// error is returned so the caller can decide how to degrade.
func (r *RedisStore) List() ([]string, error) {
	items, err := r.c.LRange(listKey, 0, -1)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []string{}
	}
	return items, nil
}

func (r *RedisStore) Add(item string) error {
	_, err := r.c.LPush(listKey, item)
	return err
}

func (r *RedisStore) Ping() error     { return r.c.Ping() }
func (r *RedisStore) Backend() string { return "redis" }
