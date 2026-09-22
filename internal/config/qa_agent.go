package config

import (
	"fmt"
	"net/url"
	"strings"
)

// QAAgentConfig registers an external QA agent that may report workflow
// go/no-go and environment health into this Ring Promoter instance.
//
// The agent lives in another repository / service: it observes CI workflows
// and ring health, then POSTs status here. Ring Promoter stores the latest
// report and surfaces it in the UI. Absent (or empty name) = integration off;
// /api/qa stays inert and the UI hides the strip — V1 behaviour unchanged.
//
// This is deliberately push-in (like CI seed and QA sign-offs), not a pull
// from the agent URL. The optional URL is only a deep-link for operators.
type QAAgentConfig struct {
	// Name is the agent identity shown in the UI and recorded on every report
	// (e.g. "qa-bot"). Required when the block is present.
	Name string `yaml:"name"`
	// URL is an optional deep-link to the agent's own UI/dashboard.
	URL string `yaml:"url"`
	// Apps scopes which applications the agent may report on. Empty means
	// every app configured on this instance.
	Apps []string `yaml:"apps"`
}

// Enabled reports whether a QA agent is registered.
func (q *QAAgentConfig) Enabled() bool {
	return q != nil && strings.TrimSpace(q.Name) != ""
}

// Watches reports whether the agent is allowed to report on app.
// When Apps is empty every configured app is in scope.
func (q *QAAgentConfig) Watches(app string) bool {
	if !q.Enabled() {
		return false
	}
	if len(q.Apps) == 0 {
		return true
	}
	for _, a := range q.Apps {
		if a == app {
			return true
		}
	}
	return false
}

// validateQAAgent checks the optional top-level qa_agent block.
func (c *Config) validateQAAgent() error {
	q := c.QAAgent
	if q == nil {
		return nil
	}
	// An empty block (just `qa_agent:`) is treated as absent — keep V1 quiet.
	if strings.TrimSpace(q.Name) == "" && strings.TrimSpace(q.URL) == "" && len(q.Apps) == 0 {
		c.QAAgent = nil
		return nil
	}
	if strings.TrimSpace(q.Name) == "" {
		return fmt.Errorf("qa_agent.name is required when qa_agent is configured")
	}
	if u := strings.TrimSpace(q.URL); u != "" {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("qa_agent.url %q must be an http(s) URL", q.URL)
		}
	}
	known := map[string]bool{}
	for _, a := range c.Apps {
		known[a.Name] = true
	}
	for _, name := range q.Apps {
		if name == "" {
			return fmt.Errorf("qa_agent.apps contains an empty name")
		}
		if !known[name] {
			return fmt.Errorf("qa_agent.apps references unknown application %q", name)
		}
	}
	return nil
}
