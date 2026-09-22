package promoter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// ErrInvalidQAReport is returned when a QA agent report fails validation.
var ErrInvalidQAReport = errors.New("invalid qa report")

// ErrQAAgentDisabled is returned when no qa_agent block is configured.
var ErrQAAgentDisabled = errors.New("qa agent is not configured")

// ErrQAAgentScope is returned when the agent posts for an app outside its
// configured apps list.
var ErrQAAgentScope = errors.New("app is outside qa agent scope")

// RecordQAReport validates and stores a status push from the configured QA
// agent. workflow_verdict is required; env_healthy is optional.
func (p *Promoter) RecordQAReport(ctx context.Context, app, ringName, workflowVerdict string, envHealthy *bool, summary, detail string, checkedAt time.Time) (store.QAReport, error) {
	qa := p.cfg.QAAgent
	if !qa.Enabled() {
		return store.QAReport{}, ErrQAAgentDisabled
	}
	ac, ok := p.cfg.App(app)
	if !ok {
		return store.QAReport{}, ErrAppNotFound
	}
	if !qa.Watches(app) {
		return store.QAReport{}, ErrQAAgentScope
	}
	ringName = strings.TrimSpace(ringName)
	if ringName != "" {
		if !ring.IsValid(ringName) {
			return store.QAReport{}, ErrRingNotConfigured
		}
		if _, ok := ac.Rings[ringName]; !ok {
			return store.QAReport{}, ErrRingNotConfigured
		}
	}
	verdict := strings.ToLower(strings.TrimSpace(workflowVerdict))
	switch verdict {
	case store.QAVerdictGo, store.QAVerdictCheck, store.QAVerdictNoGo, store.QAVerdictUnknown:
	default:
		return store.QAReport{}, fmt.Errorf("%w: workflow_verdict must be go, check, no_go or unknown", ErrInvalidQAReport)
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	report := store.QAReport{
		App:             app,
		Ring:            ringName,
		WorkflowVerdict: verdict,
		EnvHealthy:      envHealthy,
		Summary:         strings.TrimSpace(summary),
		Detail:          strings.TrimSpace(detail),
		Source:          strings.TrimSpace(qa.Name),
		CheckedAt:       checkedAt.UTC(),
	}
	if err := p.store.UpsertQAReport(ctx, report); err != nil {
		return store.QAReport{}, err
	}
	detailMap := map[string]string{
		"workflow_verdict": report.WorkflowVerdict,
		"summary":          report.Summary,
		"source":           report.Source,
	}
	if envHealthy != nil {
		if *envHealthy {
			detailMap["env_healthy"] = "true"
		} else {
			detailMap["env_healthy"] = "false"
		}
	}
	p.audit(ctx, store.AuditEvent{
		ActorType: store.ActorAgent,
		Actor:     report.Source,
		App:       app,
		Ring:      ringName,
		Category:  store.AuditConfig,
		Action:    "qa_report.record",
		Detail:    auditDetail(detailMap),
	})
	list, err := p.store.ListQAReports(ctx, app)
	if err != nil {
		return report, nil
	}
	for _, got := range list {
		if got.App == app && got.Ring == ringName {
			return got, nil
		}
	}
	return report, nil
}

// ListQAReports returns stored QA agent reports. Empty app lists every report.
func (p *Promoter) ListQAReports(ctx context.Context, app string) ([]store.QAReport, error) {
	if app != "" {
		if _, ok := p.cfg.App(app); !ok {
			return nil, ErrAppNotFound
		}
	}
	return p.store.ListQAReports(ctx, app)
}

// QAAgentEnabled reports whether a QA agent is registered in config.
func (p *Promoter) QAAgentEnabled() bool {
	return p.cfg.QAAgent.Enabled()
}

// QAAgentName returns the configured agent name, or "".
func (p *Promoter) QAAgentName() string {
	if p.cfg.QAAgent == nil {
		return ""
	}
	return strings.TrimSpace(p.cfg.QAAgent.Name)
}

// QAAgentURL returns the configured agent deep-link, or "".
func (p *Promoter) QAAgentURL() string {
	if p.cfg.QAAgent == nil {
		return ""
	}
	return strings.TrimSpace(p.cfg.QAAgent.URL)
}
