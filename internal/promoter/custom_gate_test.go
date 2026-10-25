package promoter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/changerequest"
	"github.com/example/ring-promoter/internal/config"
)

func f64(v float64) *float64 { return &v }
func iPtr(v int) *int        { return &v }

type stubInspector struct {
	tkt changerequest.Ticket
	err error
}

func (s stubInspector) Inspect(_ context.Context, _ string, _ changerequest.InspectOpts) (changerequest.Ticket, error) {
	return s.tkt, s.err
}

func TestCustomGate_MetricThreshold(t *testing.T) {
	now := mustNow()
	pol := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "traffic", Kind: config.CustomGateMetric, Rings: []string{"test"},
			Metric: &config.MetricRule{Op: "lt", Threshold: 100, Unit: "rps", DemoValue: f64(40)},
		}},
	}
	p, _, _ := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("40 rps < 100 should hop: %v", err)
	}

	p2, dep, _ := gatedHarness(t, now, &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "traffic", Kind: config.CustomGateMetric, Rings: []string{"test"},
			Metric: &config.MetricRule{Op: "lt", Threshold: 100, Unit: "rps", DemoValue: f64(150)},
		}},
	})
	mustSeed(t, p2, "int", "v1")
	before := dep.deployCount()
	if _, err := p2.Promote(ctx, testApp, "int"); !errors.Is(err, ErrCustomGate) {
		t.Fatalf("150 rps should block, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("deployed despite traffic gate")
	}
}

func TestCustomGate_ReplicasAndGradual(t *testing.T) {
	now := mustNow()
	ctx := context.Background()
	block := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "ha", Kind: config.CustomGateReplicas, Rings: []string{"test"},
			Replicas: &config.ReplicaRule{MinReady: 2, DemoReady: iPtr(1), Gradual: &config.GradualRollout{}},
		}},
	}
	p, dep, _ := gatedHarness(t, now, block)
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrCustomGate) {
		t.Fatalf("1 replica should block, got %v", err)
	}
	if dep.sawGradual {
		t.Fatal("gradual patch must not apply on a blocked hop")
	}

	ok := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "ha", Kind: config.CustomGateReplicas, Rings: []string{"test"},
			Replicas: &config.ReplicaRule{
				MinReady: 2, DemoReady: iPtr(3),
				Gradual: &config.GradualRollout{MaxUnavailable: "0", MaxSurge: "1"},
			},
		}},
	}
	p2, dep2, _ := gatedHarness(t, now, ok)
	mustSeed(t, p2, "int", "v1")
	if _, err := p2.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("3 replicas should hop: %v", err)
	}
	if !dep2.sawGradual || dep2.lastGradual.MaxUnavailable != "0" || dep2.lastGradual.MaxSurge != "1" {
		t.Fatalf("want gradual 0/1, got %+v saw=%v", dep2.lastGradual, dep2.sawGradual)
	}
}

func TestCustomGate_JIRAApprovalsAndVersion(t *testing.T) {
	now := mustNow()
	ctx := context.Background()
	pol := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "cab", Kind: config.CustomGateJIRA, Rings: []string{"test"},
			MinApprovals: 3, VersionField: "customfield_1",
			JIRA: &config.JIRAConfig{BaseURL: "https://jira.example", Email: "bot@example.com"},
		}},
	}
	p, _, _ := gatedHarness(t, now, pol)
	p.SetCustomInspector(testApp, "cab", stubInspector{tkt: changerequest.Ticket{
		Code: "CAB-1", Status: "Approved", Version: "v1", Approvals: 2,
	}})
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ChangeRequestCode: "CAB-1"}), testApp, "int"); !errors.Is(err, ErrCustomGate) {
		t.Fatalf("2 of 3 approvals should block, got %v", err)
	}

	p.SetCustomInspector(testApp, "cab", stubInspector{tkt: changerequest.Ticket{
		Code: "CAB-1", Status: "Approved", Version: "v1", Approvals: 3,
	}})
	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ChangeRequestCode: "CAB-1"}), testApp, "int"); err != nil {
		t.Fatalf("3 approvals matching version should hop: %v", err)
	}
}

func TestCustomGate_OpsAPIVersionMustMatch(t *testing.T) {
	now := mustNow()
	ctx := context.Background()
	pol := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "sr", Kind: config.CustomGateOpsAPI, Rings: []string{"test"},
			OpsAPI: &config.OpsAPIConfig{BaseURL: "https://opsapi.example"},
		}},
	}
	p, _, _ := gatedHarness(t, now, pol)
	p.SetCustomInspector(testApp, "sr", stubInspector{tkt: changerequest.Ticket{
		Code: "SR-9", Status: "approved", Version: "old",
	}})
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ServiceRequestCode: "SR-9"}), testApp, "int"); !errors.Is(err, ErrCustomGate) {
		t.Fatalf("mismatched SR version should block, got %v", err)
	}
	p.SetCustomInspector(testApp, "sr", stubInspector{tkt: changerequest.Ticket{
		Code: "SR-9", Status: "approved", Version: "v1",
	}})
	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ServiceRequestCode: "SR-9"}), testApp, "int"); err != nil {
		t.Fatalf("matching SR should hop: %v", err)
	}
}

func TestCustomGate_DemoCode(t *testing.T) {
	now := mustNow()
	pol := &config.PromotionPolicy{
		CustomGates: []config.CustomGate{{
			Name: "paper", Kind: config.CustomGateTest, Rings: []string{"test"},
		}},
	}
	p, _, _ := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrCustomGate) {
		t.Fatalf("missing demo code should block, got %v", err)
	}
	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ChangeRequestCode: "test"}), testApp, "int"); err != nil {
		t.Fatalf("demo code should hop: %v", err)
	}
}

func mustNow() time.Time {
	return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
}
