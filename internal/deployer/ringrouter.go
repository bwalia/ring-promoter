package deployer

import "context"

// RingRouter returns a Deployer that sends each ring to its own deployer
// (byRing), and every other ring to def. It backs the per-ring `deployer:`
// override, so an app can move one ring at a time to a new mechanism.
//
// Optional capabilities: VersionSource is app-level (it lists and validates the
// app's versions, not a ring's), so the router advertises it exactly when def
// does and forwards to def. LiveVersioner is not forwarded; no ring-mixable
// deployer (github, k8sjob) implements it.
func RingRouter(def Deployer, byRing map[string]Deployer) Deployer {
	r := ringRouter{def: def, byRing: byRing}
	if vs, ok := def.(VersionSource); ok {
		return ringRouterVS{ringRouter: r, vs: vs}
	}
	return r
}

type ringRouter struct {
	def    Deployer
	byRing map[string]Deployer
}

func (r ringRouter) pick(ring string) Deployer {
	if d, ok := r.byRing[ring]; ok && d != nil {
		return d
	}
	return r.def
}

func (r ringRouter) Deploy(ctx context.Context, t Target, version string) error {
	return r.pick(t.Ring).Deploy(ctx, t, version)
}

func (r ringRouter) Restart(ctx context.Context, t Target, req RestartRequest) error {
	return r.pick(t.Ring).Restart(ctx, t, req)
}

func (r ringRouter) ValidateRestart(t Target, req RestartRequest) error {
	return r.pick(t.Ring).ValidateRestart(t, req)
}

type ringRouterVS struct {
	ringRouter
	vs VersionSource
}

func (r ringRouterVS) ListVersions(ctx context.Context) ([]Version, error) {
	return r.vs.ListVersions(ctx)
}

func (r ringRouterVS) ValidateVersion(ctx context.Context, version string) error {
	return r.vs.ValidateVersion(ctx, version)
}
