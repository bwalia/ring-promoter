package promoter

import (
	"context"
	"encoding/json"

	"github.com/example/ring-promoter/internal/store"
)

// audit appends one event to the audit ledger, filling actor and correlation
// id from the operation context. Auditing must never fail the operation it
// describes, so errors are logged and swallowed (the same stance record()
// takes for history).
func (p *Promoter) audit(ctx context.Context, e store.AuditEvent) {
	a := actorFrom(ctx)
	if e.ActorType == "" {
		e.ActorType = a.Type
	}
	if e.Actor == "" {
		e.Actor = a.Name
	}
	if e.CorrelationID == "" {
		e.CorrelationID = correlationFrom(ctx)
	}
	if err := p.store.AppendAudit(ctx, e); err != nil {
		p.log.Error("append audit failed", "err", err,
			"category", e.Category, "action", e.Action, "app", e.App, "ring", e.Ring)
	}
}

// auditDetail JSON-encodes a detail map for an audit event. Encoding a plain
// map[string]string cannot fail; the fallback keeps audit non-fatal anyway.
func auditDetail(kv map[string]string) string {
	b, err := json.Marshal(kv)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Audit returns audit-ledger events matching the filter, newest first.
func (p *Promoter) Audit(ctx context.Context, f store.AuditFilter) ([]store.AuditEvent, error) {
	return p.store.ListAudit(ctx, f)
}
