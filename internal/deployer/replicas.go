package deployer

import "context"

// ReplicaReader reports how many pods are Ready on the target Deployment.
// KubectlDeployer implements it; deployers without Kubernetes return an error
// and a replica custom-gate must then use demo_ready.
type ReplicaReader interface {
	ReadyReplicas(ctx context.Context, t Target) (int, error)
}

// GradualRollout is a Kubernetes rollingUpdate applied for one hop so a
// multi-replica promotion is controlled (maxUnavailable/maxSurge) rather than
// a surge of new pods.
type GradualRollout struct {
	MaxUnavailable string
	MaxSurge       string
}

type gradualKey struct{}

// WithGradualRollout stashes a rollingUpdate on the deploy context. Kubectl
// Deploy reads it before setting the image.
func WithGradualRollout(ctx context.Context, g GradualRollout) context.Context {
	if g.MaxUnavailable == "" && g.MaxSurge == "" {
		return ctx
	}
	return context.WithValue(ctx, gradualKey{}, g)
}

// GradualRolloutFrom returns the hop's rollingUpdate, if any.
func GradualRolloutFrom(ctx context.Context) (GradualRollout, bool) {
	g, ok := ctx.Value(gradualKey{}).(GradualRollout)
	return g, ok
}
