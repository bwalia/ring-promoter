package config

import (
	"fmt"
	"strings"
	"time"
)

// Pipeline stage types.
const (
	StageDeploy   = "deploy"
	StageGate     = "gate"
	StageApproval = "approval"
)

// GateQA is the only gate kind today: the external QA agent's go/no-go for
// each app on the run's ring.
const GateQA = "qa"

// Default waits for stages that wait on something outside Ring Promoter.
const (
	DefaultGateTimeout     = 10 * time.Minute
	DefaultApprovalTimeout = 24 * time.Hour
)

// PipelineConfig is a release workflow across several applications: an
// ordered DAG of stages (à la CircleCI workflows / GitHub Actions jobs), each
// deploying a set of apps in parallel, checking a QA gate, or holding for a
// human approval. A run takes every deploy stage's apps into one ring.
type PipelineConfig struct {
	Name        string          `yaml:"name"`
	DisplayName string          `yaml:"display_name"`
	Description string          `yaml:"description"`
	Stages      []PipelineStage `yaml:"stages"`
}

// PipelineStage is one node of a pipeline.
type PipelineStage struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
	// Type is deploy (default), gate or approval.
	Type string `yaml:"type"`
	// Apps are deployed in parallel (deploy) or checked (gate). A gate with no
	// apps checks every app its direct needs deploy. Approvals take none.
	Apps []string `yaml:"apps"`
	// Needs names the stages that must succeed before this one starts.
	Needs []string `yaml:"needs"`
	// Gate is the gate kind for a gate stage; only "qa" (the default).
	Gate string `yaml:"gate"`
	// Timeout bounds a gate's wait for a verdict or an approval's wait for a
	// decision. Defaults: gate 10m, approval 24h.
	Timeout Duration `yaml:"timeout"`
}

// ResolvedType returns the stage type, defaulting to deploy.
func (s PipelineStage) ResolvedType() string {
	if s.Type == "" {
		return StageDeploy
	}
	return s.Type
}

// ResolvedGate returns the gate kind, defaulting to qa.
func (s PipelineStage) ResolvedGate() string {
	if s.Gate == "" {
		return GateQA
	}
	return s.Gate
}

// ResolvedTimeout returns the stage's wait bound with its type's default.
func (s PipelineStage) ResolvedTimeout() time.Duration {
	if t := s.Timeout.Std(); t > 0 {
		return t
	}
	if s.ResolvedType() == StageApproval {
		return DefaultApprovalTimeout
	}
	return DefaultGateTimeout
}

// Title returns the display name, falling back to the name.
func (s PipelineStage) Title() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}

// Title returns the display name, falling back to the name.
func (p PipelineConfig) Title() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Name
}

// Pipeline returns the named pipeline.
func (c *Config) Pipeline(name string) (PipelineConfig, bool) {
	for _, p := range c.Pipelines {
		if p.Name == name {
			return p, true
		}
	}
	return PipelineConfig{}, false
}

// validatePipelines checks the optional top-level pipelines block: unique
// names, known stage types and apps, and needs that form a DAG.
func (c *Config) validatePipelines() error {
	known := map[string]bool{}
	for _, a := range c.Apps {
		known[a.Name] = true
	}
	seen := map[string]bool{}
	for _, p := range c.Pipelines {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("pipeline with empty name")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate pipeline %q", p.Name)
		}
		seen[p.Name] = true
		if len(p.Stages) == 0 {
			return fmt.Errorf("pipeline %q has no stages", p.Name)
		}
		stages := map[string]PipelineStage{}
		for _, s := range p.Stages {
			where := fmt.Sprintf("pipeline %q stage %q", p.Name, s.Name)
			if strings.TrimSpace(s.Name) == "" {
				return fmt.Errorf("pipeline %q has a stage with an empty name", p.Name)
			}
			if _, dup := stages[s.Name]; dup {
				return fmt.Errorf("pipeline %q has duplicate stage %q", p.Name, s.Name)
			}
			stages[s.Name] = s
			if s.Timeout.Std() < 0 {
				return fmt.Errorf("%s: timeout must not be negative", where)
			}
			switch s.ResolvedType() {
			case StageDeploy:
				if len(s.Apps) == 0 {
					return fmt.Errorf("%s: a deploy stage needs at least one app", where)
				}
			case StageGate:
				if s.ResolvedGate() != GateQA {
					return fmt.Errorf("%s: unknown gate %q (want %q)", where, s.Gate, GateQA)
				}
			case StageApproval:
				if len(s.Apps) > 0 {
					return fmt.Errorf("%s: an approval stage takes no apps", where)
				}
			default:
				return fmt.Errorf("%s: unknown type %q (want %q, %q or %q)", where, s.Type, StageDeploy, StageGate, StageApproval)
			}
			apps := map[string]bool{}
			for _, a := range s.Apps {
				if !known[a] {
					return fmt.Errorf("%s: unknown application %q", where, a)
				}
				if apps[a] {
					return fmt.Errorf("%s: application %q listed twice", where, a)
				}
				apps[a] = true
			}
		}
		for _, s := range p.Stages {
			for _, n := range s.Needs {
				if _, ok := stages[n]; !ok {
					return fmt.Errorf("pipeline %q stage %q needs unknown stage %q", p.Name, s.Name, n)
				}
				if n == s.Name {
					return fmt.Errorf("pipeline %q stage %q must not need itself", p.Name, s.Name)
				}
			}
		}
		if cyc := pipelineCycle(p.Stages); cyc != "" {
			return fmt.Errorf("pipeline %q has a dependency cycle through stage %q", p.Name, cyc)
		}
	}
	return nil
}

// pipelineCycle returns a stage on a needs cycle, or "" when the stages form
// a DAG (depth-first search with white/grey/black colouring).
func pipelineCycle(stages []PipelineStage) string {
	needs := map[string][]string{}
	for _, s := range stages {
		needs[s.Name] = s.Needs
	}
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var visit func(n string) string
	visit = func(n string) string {
		color[n] = grey
		for _, m := range needs[n] {
			switch color[m] {
			case grey:
				return m
			case white:
				if c := visit(m); c != "" {
					return c
				}
			}
		}
		color[n] = black
		return ""
	}
	for _, s := range stages {
		if color[s.Name] == white {
			if c := visit(s.Name); c != "" {
				return c
			}
		}
	}
	return ""
}
