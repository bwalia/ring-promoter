package promoter

import (
	"context"
	"strings"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// QA verdicts a pipeline gate reads (store.QAReport.WorkflowVerdict).
const (
	QAVerdictGo      = "go"
	QAVerdictCheck   = "check"
	QAVerdictNoGo    = "no_go"
	QAVerdictUnknown = "unknown"
)

// Pipelines returns the configured multi-app pipelines, in config order.
func (p *Promoter) Pipelines() []config.PipelineConfig {
	return p.cfg.Pipelines
}

// Pipeline returns the named pipeline.
func (p *Promoter) Pipeline(name string) (config.PipelineConfig, bool) {
	return p.cfg.Pipeline(name)
}

// RingRef returns the version the app's ring is pinned to in config (the
// ring's `ref`), or "" when it is not pinned or not configured.
func (p *Promoter) RingRef(app, ringName string) string {
	a, ok := p.cfg.App(app)
	if !ok {
		return ""
	}
	return strings.TrimSpace(a.Rings[ringName].Ref)
}

// QAVerdict returns the QA agent's latest workflow verdict for app on ring
// (falling back to its app-wide report) and the report's summary. Without a
// report the verdict is "unknown".
func (p *Promoter) QAVerdict(ctx context.Context, app, ringName string) (verdict, summary string, err error) {
	reports, err := p.store.ListQAReports(ctx, app)
	if err != nil {
		return QAVerdictUnknown, "", err
	}
	var wide *store.QAReport
	for i := range reports {
		r := &reports[i]
		if r.App != app {
			continue
		}
		if r.Ring == ringName {
			return normalizeVerdict(r.WorkflowVerdict), r.Summary, nil
		}
		if r.Ring == "" {
			wide = r
		}
	}
	if wide != nil {
		return normalizeVerdict(wide.WorkflowVerdict), wide.Summary, nil
	}
	return QAVerdictUnknown, "no QA report yet", nil
}

func normalizeVerdict(v string) string {
	switch v {
	case QAVerdictGo, QAVerdictCheck, QAVerdictNoGo:
		return v
	}
	return QAVerdictUnknown
}

// AuditPipeline records a pipeline run's lifecycle event (start, approve,
// reject, cancel) in the audit ledger under the request's actor.
func (p *Promoter) AuditPipeline(ctx context.Context, ringName, action string, detail map[string]string) {
	p.audit(ctx, store.AuditEvent{
		Ring: ringName, Category: store.AuditPipeline, Action: action,
		Detail: auditDetail(detail),
	})
}
