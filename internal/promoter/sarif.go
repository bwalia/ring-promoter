package promoter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// sarifLog is the subset of SARIF 2.1.0 a security gate needs: each run's
// tool, its rules and its results. Trivy, Grype, Semgrep, CodeQL, Gitleaks,
// Checkov and most other scanners emit it.
type sarifLog struct {
	Runs []struct {
		Tool struct {
			Driver struct {
				Name  string      `json:"name"`
				Rules []sarifRule `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID     string         `json:"ruleId"`
			RuleIndex  *int           `json:"ruleIndex"`
			Level      string         `json:"level"`
			Message    sarifText      `json:"message"`
			Properties map[string]any `json:"properties"`
			Locations  []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	ShortDescription     sarifText      `json:"shortDescription"`
	Properties           map[string]any `json:"properties"`
	DefaultConfiguration struct {
		Level string `json:"level"`
	} `json:"defaultConfiguration"`
}

// parseSARIF turns a SARIF document into findings and returns the first
// run's tool name. Severity comes from the "security-severity" property (on
// the result, else its rule): >=9 critical, >=7 high, >=4 medium, else low;
// without one, the SARIF level maps error->high, warning (the default)->medium
// and note/none->low. Every finding of a secrets scan is critical: a leaked
// credential is exploitable whatever the scanner calls it.
func parseSARIF(raw json.RawMessage, kind string) (findings []store.SecurityFinding, tool string, err error) {
	var log sarifLog
	if err := json.Unmarshal(raw, &log); err != nil {
		return nil, "", fmt.Errorf("%w: sarif: %v", ErrInvalidSecurityScan, err)
	}
	findings = []store.SecurityFinding{}
	for _, run := range log.Runs {
		if tool == "" {
			tool = run.Tool.Driver.Name
		}
		rules := map[string]sarifRule{}
		for _, r := range run.Tool.Driver.Rules {
			rules[r.ID] = r
		}
		for _, res := range run.Results {
			rule, ok := rules[res.RuleID]
			if !ok && res.RuleIndex != nil && *res.RuleIndex >= 0 && *res.RuleIndex < len(run.Tool.Driver.Rules) {
				rule = run.Tool.Driver.Rules[*res.RuleIndex]
			}
			id := res.RuleID
			if id == "" {
				id = rule.ID
			}
			sev := ""
			if score, ok := securitySeverity(res.Properties); ok {
				sev = severityFromScore(score)
			} else if score, ok := securitySeverity(rule.Properties); ok {
				sev = severityFromScore(score)
			} else {
				level := res.Level
				if level == "" {
					level = rule.DefaultConfiguration.Level
				}
				sev = severityFromLevel(level)
			}
			if kind == config.ScanSecrets {
				sev = config.SeverityCritical
			}
			title := rule.ShortDescription.Text
			if title == "" {
				title = firstLine(res.Message.Text)
			}
			f := store.SecurityFinding{ID: id, Severity: sev, Title: title}
			if len(res.Locations) > 0 {
				loc := res.Locations[0].PhysicalLocation
				f.Location = loc.ArtifactLocation.URI
				if loc.Region.StartLine > 0 && f.Location != "" {
					f.Location += ":" + strconv.Itoa(loc.Region.StartLine)
				}
			}
			findings = append(findings, f)
		}
	}
	return findings, tool, nil
}

func securitySeverity(props map[string]any) (float64, bool) {
	switch v := props["security-severity"].(type) {
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	case float64:
		return v, true
	}
	return 0, false
}

func severityFromScore(s float64) string {
	switch {
	case s >= 9:
		return config.SeverityCritical
	case s >= 7:
		return config.SeverityHigh
	case s >= 4:
		return config.SeverityMedium
	}
	return config.SeverityLow
}

func severityFromLevel(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return config.SeverityHigh
	case "note", "none":
		return config.SeverityLow
	}
	return config.SeverityMedium
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
