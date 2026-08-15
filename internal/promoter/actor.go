package promoter

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/example/ring-promoter/internal/store"
)

// Actor identifies who is performing an operation for the audit ledger. It is
// threaded through the operation context (like GateInputs) so Seed/Promote/
// Rollback keep their signatures. The API attaches it from the request; the
// Ring Agent will attach itself as actor_type "agent".
type Actor struct {
	// Type is one of store.ActorHuman, store.ActorAgent, store.ActorSystem.
	Type string
	// Name is the self-declared identity (e.g. the X-Actor header). There is no
	// authenticated per-user identity yet, so this is attribution, not proof.
	Name string
}

type actorKey struct{}

// WithActor returns a context carrying the operation's actor.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// actorFrom extracts the actor from the context, defaulting to an anonymous
// human so pre-existing callers (and old clients) audit exactly as before —
// attributed to nobody in particular, but still recorded.
func actorFrom(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok && a.Type != "" {
		return a
	}
	return Actor{Type: store.ActorHuman, Name: "anonymous"}
}

type correlationKey struct{}

// WithCorrelationID returns a context carrying the operation's correlation id,
// which ties history rows, the pending-op journal and every audit event of one
// operation together.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

// correlationFrom extracts the correlation id ("" if absent).
func correlationFrom(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}

// NewCorrelationID mints a random 16-hex-char correlation id.
func NewCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "" // audit rows tolerate an empty correlation id
	}
	return hex.EncodeToString(b[:])
}
