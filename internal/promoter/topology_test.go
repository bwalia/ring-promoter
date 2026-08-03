package promoter

import (
	"context"
	"testing"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/store"
)

func TestTopologyMergesConfigUserAndSuppressions(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	cfg := &config.Config{Apps: []config.AppConfig{
		{Name: "api"},
		{Name: "web", DependsOn: []string{"api"}},
		{Name: "worker"},
	}}
	p := New(cfg, st, nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)

	if err := st.AddTopologyEdge(ctx, "worker", "api"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTopologySuppression(ctx, "web", "api"); err != nil {
		t.Fatal(err)
	}
	view, err := p.Topology(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Edges) != 1 || view.Edges[0] != (TopologyEdgeView{From: "worker", To: "api", Source: "user"}) {
		t.Fatalf("effective edges = %#v, want user edge only", view.Edges)
	}

	if err := p.RestoreTopologyEdge(ctx, "web", "api"); err != nil {
		t.Fatal(err)
	}
	view, err = p.Topology(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []TopologyEdgeView{
		{From: "web", To: "api", Source: "config"},
		{From: "worker", To: "api", Source: "user"},
	}
	if len(view.Edges) != len(want) {
		t.Fatalf("effective edges = %#v, want %#v", view.Edges, want)
	}
	for i := range want {
		if view.Edges[i] != want[i] {
			t.Fatalf("edge %d = %#v, want %#v", i, view.Edges[i], want[i])
		}
	}
}
