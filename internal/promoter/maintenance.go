package promoter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// Maintenance-window / sign-off precondition errors (mapped to 4xx by the API).
var (
	ErrWindowNotFound = errors.New("maintenance window not found")
	ErrInvalidWindow  = errors.New("invalid maintenance window")
	ErrInvalidSignoff = errors.New("invalid sign-off")
)

// MaintenanceView is the aggregate read model for an app's maintenance windows:
// the config-defined recurring windows, the operator-created ad-hoc windows,
// which rings the gate guards, and whether each guarded ring is open right now.
type MaintenanceView struct {
	// Gated reports whether the app gates any ring behind a maintenance window.
	Gated bool `json:"gated"`
	// GatedRings are the configured rings the gate guards.
	GatedRings []string `json:"gated_rings"`
	// Recurring are the permanent windows from config.
	Recurring []config.RecurringWindow `json:"recurring"`
	// Windows are the operator-created ad-hoc windows, newest first.
	Windows []store.MaintenanceWindow `json:"windows"`
	// OpenRings maps each guarded ring to whether a window is open now.
	OpenRings map[string]bool `json:"open_rings"`
}

// CreateMaintenanceWindow validates and stores an operator-created ad-hoc
// window. ring may be empty ("all guarded rings") or a configured ring name.
func (p *Promoter) CreateMaintenanceWindow(ctx context.Context, app, ringName string, startsAt, endsAt time.Time, reason, createdBy string) (store.MaintenanceWindow, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return store.MaintenanceWindow{}, ErrAppNotFound
	}
	if ringName != "" {
		if !ring.IsValid(ringName) {
			return store.MaintenanceWindow{}, ErrRingNotConfigured
		}
		if _, ok := ac.Rings[ringName]; !ok {
			return store.MaintenanceWindow{}, ErrRingNotConfigured
		}
	}
	if !endsAt.After(startsAt) {
		return store.MaintenanceWindow{}, fmt.Errorf("%w: end must be after start", ErrInvalidWindow)
	}
	id, err := randomID("mw-")
	if err != nil {
		return store.MaintenanceWindow{}, err
	}
	w := store.MaintenanceWindow{
		ID: id, App: app, Ring: ringName,
		StartsAt: startsAt.UTC(), EndsAt: endsAt.UTC(),
		Reason: strings.TrimSpace(reason), CreatedBy: strings.TrimSpace(createdBy),
	}
	if err := p.store.CreateMaintenanceWindow(ctx, w); err != nil {
		return store.MaintenanceWindow{}, err
	}
	// Return the stored form (with CreatedAt stamped) for the response.
	if list, err := p.store.ListMaintenanceWindows(ctx, app); err == nil {
		for _, e := range list {
			if e.ID == id {
				return e, nil
			}
		}
	}
	return w, nil
}

// MaintenanceWindows returns the aggregate maintenance view for an app.
func (p *Promoter) MaintenanceWindows(ctx context.Context, app string) (MaintenanceView, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return MaintenanceView{}, ErrAppNotFound
	}
	windows, err := p.store.ListMaintenanceWindows(ctx, app)
	if err != nil {
		return MaintenanceView{}, err
	}
	if windows == nil {
		windows = []store.MaintenanceWindow{}
	}
	view := MaintenanceView{Windows: windows, OpenRings: map[string]bool{}}

	if ac.PromotionPolicy == nil || ac.PromotionPolicy.MaintenanceWindow == nil {
		return view, nil
	}
	mw := ac.PromotionPolicy.MaintenanceWindow
	view.Gated = true
	view.Recurring = mw.Recurring
	now := p.now()
	// Guarded rings, in pipeline order, restricted to those the app configures.
	for _, r := range ring.Names() {
		if _, configured := ac.Rings[r]; !configured {
			continue
		}
		if !mw.Guards(r) {
			continue
		}
		view.GatedRings = append(view.GatedRings, r)
		open, err := p.maintenanceOpenAt(ctx, app, r, now)
		if err != nil {
			return MaintenanceView{}, err
		}
		view.OpenRings[r] = open
	}
	return view, nil
}

// DeleteMaintenanceWindow removes one of an app's ad-hoc windows.
func (p *Promoter) DeleteMaintenanceWindow(ctx context.Context, app, id string) error {
	if _, ok := p.cfg.App(app); !ok {
		return ErrAppNotFound
	}
	if err := p.store.DeleteMaintenanceWindow(ctx, app, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrWindowNotFound
		}
		return err
	}
	return nil
}

// RecordSignoff validates and stores a QA/release Go-No-Go decision for one
// exact (app, ring, version). A release engineer must be named.
func (p *Promoter) RecordSignoff(ctx context.Context, app, ringName, version, decision, engineer, qaStatus, note string) (store.Signoff, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return store.Signoff{}, ErrAppNotFound
	}
	if !ring.IsValid(ringName) {
		return store.Signoff{}, ErrRingNotConfigured
	}
	if _, ok := ac.Rings[ringName]; !ok {
		return store.Signoff{}, ErrRingNotConfigured
	}
	if strings.TrimSpace(version) == "" {
		return store.Signoff{}, fmt.Errorf("%w: version is required", ErrInvalidSignoff)
	}
	switch decision {
	case store.DecisionGo, store.DecisionNoGo:
	default:
		return store.Signoff{}, fmt.Errorf("%w: decision must be %q or %q", ErrInvalidSignoff, store.DecisionGo, store.DecisionNoGo)
	}
	if strings.TrimSpace(engineer) == "" {
		return store.Signoff{}, fmt.Errorf("%w: the release engineer's name is required", ErrInvalidSignoff)
	}
	s := store.Signoff{
		App: app, Ring: ringName, Version: strings.TrimSpace(version),
		Decision: decision, Engineer: strings.TrimSpace(engineer),
		QAStatus: strings.TrimSpace(qaStatus), Note: strings.TrimSpace(note),
	}
	if err := p.store.UpsertSignoff(ctx, s); err != nil {
		return store.Signoff{}, err
	}
	if got, err := p.store.GetSignoff(ctx, app, ringName, s.Version); err == nil {
		return got, nil
	}
	return s, nil
}

// Signoffs returns an app's sign-offs, newest first.
func (p *Promoter) Signoffs(ctx context.Context, app string) ([]store.Signoff, error) {
	if _, ok := p.cfg.App(app); !ok {
		return nil, ErrAppNotFound
	}
	return p.store.ListSignoffs(ctx, app)
}

// randomID returns a short random identifier with the given prefix.
func randomID(prefix string) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}
