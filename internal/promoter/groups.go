package promoter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/example/ring-promoter/internal/store"
)

// Group precondition errors (mapped to 4xx by the API layer).
var (
	ErrEmptyGroupName = errors.New("group name must not be empty")
	ErrUnknownApp     = errors.New("unknown application")
	ErrGroupNotFound  = errors.New("group not found")
)

// Groups returns every saved application group. Groups are stored server-side
// so they are shared by all users and survive browser changes.
func (p *Promoter) Groups(ctx context.Context) ([]store.Group, error) {
	return p.store.ListGroups(ctx)
}

// CreateGroup validates and stores a new group, returning it with its
// server-assigned ID.
func (p *Promoter) CreateGroup(ctx context.Context, name string, apps []string) (store.Group, error) {
	g := store.Group{Name: strings.TrimSpace(name)}
	var err error
	if g.Apps, err = p.validateGroup(g.Name, apps); err != nil {
		return store.Group{}, err
	}
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return store.Group{}, fmt.Errorf("generate group id: %w", err)
	}
	g.ID = "g-" + hex.EncodeToString(buf)
	if err := p.store.CreateGroup(ctx, g); err != nil {
		return store.Group{}, err
	}
	return g, nil
}

// UpdateGroup validates and replaces an existing group's name and members.
func (p *Promoter) UpdateGroup(ctx context.Context, id, name string, apps []string) (store.Group, error) {
	g := store.Group{ID: id, Name: strings.TrimSpace(name)}
	var err error
	if g.Apps, err = p.validateGroup(g.Name, apps); err != nil {
		return store.Group{}, err
	}
	if err := p.store.UpdateGroup(ctx, g); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.Group{}, ErrGroupNotFound
		}
		return store.Group{}, err
	}
	return g, nil
}

// DeleteGroup removes a group.
func (p *Promoter) DeleteGroup(ctx context.Context, id string) error {
	if err := p.store.DeleteGroup(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrGroupNotFound
		}
		return err
	}
	return nil
}

// validateGroup checks the name and that every member is a configured app,
// returning the members deduplicated with order preserved.
func (p *Promoter) validateGroup(name string, apps []string) ([]string, error) {
	if name == "" {
		return nil, ErrEmptyGroupName
	}
	known := make(map[string]bool, len(p.cfg.Apps))
	for _, a := range p.cfg.Apps {
		known[a.Name] = true
	}
	out := make([]string, 0, len(apps))
	seen := make(map[string]bool, len(apps))
	for _, a := range apps {
		if !known[a] {
			return nil, fmt.Errorf("%w: %q", ErrUnknownApp, a)
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out, nil
}
