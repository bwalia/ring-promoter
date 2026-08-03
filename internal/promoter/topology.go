package promoter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Topology mutation precondition errors.
var (
	ErrSelfDependency = errors.New("application must not depend on itself")
	ErrEdgeNotFound   = errors.New("topology edge not found")
	ErrEmptyEdge      = errors.New("topology edge endpoints must not be empty")
)

// TopologyEdgeView is one effective dashboard edge and where it originated.
type TopologyEdgeView struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Source string `json:"source"` // "config" | "user"
}

// TopologyView is the effective topology: config and user edges, minus
// suppressions.
type TopologyView struct {
	Edges []TopologyEdgeView `json:"edges"`
}

// Topology returns the effective edges, combining configuration-owned and
// user-defined edges while removing suppressed configuration edges.
func (p *Promoter) Topology(ctx context.Context) (TopologyView, error) {
	userEdges, err := p.store.ListTopologyEdges(ctx)
	if err != nil {
		return TopologyView{}, err
	}
	suppressions, err := p.store.ListTopologySuppressions(ctx)
	if err != nil {
		return TopologyView{}, err
	}

	edges := make(map[string]TopologyEdgeView)
	for _, app := range p.cfg.Apps {
		for _, dependency := range app.DependsOn {
			edges[topologyEdgeKey(app.Name, dependency)] = TopologyEdgeView{
				From: app.Name, To: dependency, Source: "config",
			}
		}
	}
	for _, edge := range userEdges {
		key := topologyEdgeKey(edge.From, edge.To)
		if _, configOwned := edges[key]; !configOwned {
			edges[key] = TopologyEdgeView{From: edge.From, To: edge.To, Source: "user"}
		}
	}
	for _, edge := range suppressions {
		delete(edges, topologyEdgeKey(edge.From, edge.To))
	}

	out := make([]TopologyEdgeView, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return TopologyView{Edges: out}, nil
}

// AddTopologyEdge adds a user edge, or restores a configuration edge previously
// suppressed by an operator.
func (p *Promoter) AddTopologyEdge(ctx context.Context, from, to string) error {
	from, to, err := p.validateTopologyEdge(from, to)
	if err != nil {
		return err
	}
	if p.isConfigTopologyEdge(from, to) {
		return p.store.DeleteTopologySuppression(ctx, from, to)
	}
	return p.store.AddTopologyEdge(ctx, from, to)
}

// RemoveTopologyEdge removes a user edge. Configuration edges are instead
// suppressed so the declaration remains intact and can later be restored.
func (p *Promoter) RemoveTopologyEdge(ctx context.Context, from, to string) error {
	from, to, err := p.validateTopologyEdge(from, to)
	if err != nil {
		return err
	}
	userEdges, err := p.store.ListTopologyEdges(ctx)
	if err != nil {
		return err
	}
	for _, edge := range userEdges {
		if edge.From == from && edge.To == to {
			return p.store.DeleteTopologyEdge(ctx, from, to)
		}
	}
	if p.isConfigTopologyEdge(from, to) {
		return p.store.AddTopologySuppression(ctx, from, to)
	}
	return ErrEdgeNotFound
}

// RestoreTopologyEdge clears a suppression for a configuration edge.
func (p *Promoter) RestoreTopologyEdge(ctx context.Context, from, to string) error {
	from, to, err := p.validateTopologyEdge(from, to)
	if err != nil {
		return err
	}
	if !p.isConfigTopologyEdge(from, to) {
		return ErrEdgeNotFound
	}
	return p.store.DeleteTopologySuppression(ctx, from, to)
}

func (p *Promoter) validateTopologyEdge(from, to string) (string, string, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return "", "", ErrEmptyEdge
	}
	if from == to {
		return "", "", ErrSelfDependency
	}
	if _, ok := p.cfg.App(from); !ok {
		return "", "", fmt.Errorf("%w: %q", ErrUnknownApp, from)
	}
	if _, ok := p.cfg.App(to); !ok {
		return "", "", fmt.Errorf("%w: %q", ErrUnknownApp, to)
	}
	return from, to, nil
}

func (p *Promoter) isConfigTopologyEdge(from, to string) bool {
	app, ok := p.cfg.App(from)
	if !ok {
		return false
	}
	for _, dependency := range app.DependsOn {
		if dependency == to {
			return true
		}
	}
	return false
}

func topologyEdgeKey(from, to string) string { return from + "\x00" + to }
