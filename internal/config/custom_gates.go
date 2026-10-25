package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/example/ring-promoter/internal/ring"
)

// Custom gate kinds an operator can attach to a promotion_policy.
const (
	CustomGateJIRA     = "jira"
	CustomGateOpsAPI   = "opsapi"
	CustomGateTest     = "test"
	CustomGateMetric   = "metric"
	CustomGateReplicas = "replicas"
)

// Metric comparison operators.
const (
	MetricLT  = "lt"
	MetricLTE = "lte"
	MetricGT  = "gt"
	MetricGTE = "gte"
	MetricEQ  = "eq"
)

// CustomGate is one extra promotion check the operator names in config —
// a JIRA ticket that must already have N approvals, an OPSAPI service
// request whose version matches the hop, and so on. Unlike the built-in
// gates, these are a list: an app can require CAB on acc and an OPSAPI
// service request on prod without forking Ring Promoter.
type CustomGate struct {
	// Name is the audit/UI id (e.g. "cab", "opsapi_sr"). Required, unique per app.
	Name string `yaml:"name"`
	// Label is shown in the console; empty uses Name.
	Label string `yaml:"label"`
	// Kind selects the backend: jira, opsapi, or test (demo code "test" only).
	Kind string `yaml:"kind"`
	// Rings are the target rings this gate guards. Empty = acc + prod.
	Rings []string `yaml:"rings"`
	// MinApprovals, when > 0, requires that many approved approvers on the
	// ticket (Jira Service Management approvals, or OPSAPI approval count).
	MinApprovals int `yaml:"min_approvals"`
	// AllowedStatuses, when set, requires the ticket/request status to be one
	// of these (case-insensitive). Empty: any existing record is acceptable
	// for JIRA; OPSAPI defaults to "approved".
	AllowedStatuses []string `yaml:"allowed_statuses"`
	// ProjectKeys (JIRA) restricts the ticket's project prefix.
	ProjectKeys []string `yaml:"project_keys"`
	// VersionField, when set, must equal the version being promoted
	// (JIRA custom field id, or JSON path on the OPSAPI body, default
	// "version"). This is the everyday "the service request is for THIS
	// build, not last week's" check CI never makes.
	VersionField string `yaml:"version_field"`
	// JIRA is used when Kind is jira. If omitted, the app's change_request.jira
	// block is reused so operators do not duplicate credentials.
	JIRA *JIRAConfig `yaml:"jira"`
	// OpsAPI is used when Kind is opsapi.
	OpsAPI *OpsAPIConfig `yaml:"opsapi"`
	// Metric is a live number compared to a threshold — e.g. "promote to prod
	// only while site traffic is under 100 rps".
	Metric *MetricRule `yaml:"metric"`
	// Replicas reads Kubernetes readyReplicas. A single replica is a snowflake
	// deploy: there is no gradual rollout. Two or more allows a controlled
	// rolling update (maxUnavailable/maxSurge).
	Replicas *ReplicaRule `yaml:"replicas"`
}

// MetricRule compares one live number (Grafana/Prometheus, or a demo value) to
// a threshold. op is lt/lte/gt/gte/eq.
type MetricRule struct {
	// Query is PromQL or the Grafana panel expression.
	Query string `yaml:"query"`
	// DatasourceUID / DatasourceType are required when using a live Grafana
	// (reused from promotion_policy.grafana when omitted).
	DatasourceUID  string  `yaml:"datasource_uid"`
	DatasourceType string  `yaml:"datasource_type"`
	Op             string  `yaml:"op"`
	Threshold      float64 `yaml:"threshold"`
	// Unit is shown in the UI, e.g. "rps".
	Unit string `yaml:"unit"`
	// DemoValue, when set, skips Grafana and uses this number (tests / no stack).
	DemoValue *float64 `yaml:"demo_value"`
}

func (m *MetricRule) OpOr() string {
	op := strings.ToLower(strings.TrimSpace(m.Op))
	if op == "" {
		return MetricLT
	}
	return op
}

// Compare reports whether value satisfies the rule.
func (m *MetricRule) Compare(value float64) bool {
	if m == nil {
		return false
	}
	switch m.OpOr() {
	case MetricLTE:
		return value <= m.Threshold
	case MetricGT:
		return value > m.Threshold
	case MetricGTE:
		return value >= m.Threshold
	case MetricEQ:
		return value == m.Threshold
	default: // lt
		return value < m.Threshold
	}
}

func (m *MetricRule) Describe() string {
	unit := strings.TrimSpace(m.Unit)
	if unit != "" {
		unit = " " + unit
	}
	return fmt.Sprintf("%s %g%s", m.OpOr(), m.Threshold, unit)
}

// ReplicaRule gates a hop on how many pods are already serving the target ring.
type ReplicaRule struct {
	// MinReady is the fewest ready replicas required to hop. Default 2 when
	// Gradual is set, otherwise 1.
	MinReady int `yaml:"min_ready"`
	// DemoReady, when set, skips kubectl (tests / non-k8s deployers).
	DemoReady *int `yaml:"demo_ready"`
	// Gradual, when set, patches the Deployment's rollingUpdate before the
	// image change so a multi-replica hop is controlled rather than a surge.
	Gradual *GradualRollout `yaml:"gradual"`
}

func (r *ReplicaRule) MinReadyOr() int {
	if r == nil {
		return 1
	}
	if r.MinReady > 0 {
		return r.MinReady
	}
	if r.Gradual != nil {
		return 2
	}
	return 1
}

// GradualRollout is a Kubernetes rollingUpdate strategy applied for one hop.
type GradualRollout struct {
	MaxUnavailable string `yaml:"max_unavailable"`
	MaxSurge       string `yaml:"max_surge"`
}

func (g *GradualRollout) MaxUnavailableOr() string {
	if g != nil && strings.TrimSpace(g.MaxUnavailable) != "" {
		return g.MaxUnavailable
	}
	return "0"
}

func (g *GradualRollout) MaxSurgeOr() string {
	if g != nil && strings.TrimSpace(g.MaxSurge) != "" {
		return g.MaxSurge
	}
	return "1"
}

// OpsAPIConfig talks to the OpsAPI Services / service-request API. Token is
// never stored here — it comes from TokenEnv (a Secret).
type OpsAPIConfig struct {
	// BaseURL is the OpsAPI origin, e.g. "https://opsapi.workstation.co.uk".
	BaseURL string `yaml:"base_url"`
	// Path is the request collection, default "/api/v2/namespace/service-requests".
	// GET {base}{path}/{code} must return the request.
	Path string `yaml:"path"`
	// TokenEnv names the bearer-token env var. Default "RP_OPSAPI_TOKEN".
	TokenEnv string `yaml:"token_env"`
	// Namespace is sent as X-Namespace-Id (OpsAPI workspace).
	Namespace string `yaml:"namespace"`
}

func (g CustomGate) KindOr() string {
	k := strings.ToLower(strings.TrimSpace(g.Kind))
	if k == "" {
		return CustomGateTest
	}
	return k
}

func (g CustomGate) LabelOr() string {
	if s := strings.TrimSpace(g.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(g.Name); s != "" {
		return s
	}
	return g.KindOr()
}

func (g CustomGate) Guards(ringName string) bool {
	return ringInSet(ringName, gateRings(g.Rings))
}

func (g CustomGate) MatchesVersion() bool {
	return strings.TrimSpace(g.VersionField) != "" || g.KindOr() == CustomGateOpsAPI
}

func (o *OpsAPIConfig) TokenEnvName() string {
	if o != nil && strings.TrimSpace(o.TokenEnv) != "" {
		return o.TokenEnv
	}
	return "RP_OPSAPI_TOKEN"
}

func (o *OpsAPIConfig) PathOr() string {
	if o != nil && strings.TrimSpace(o.Path) != "" {
		return o.Path
	}
	return "/api/v2/namespace/service-requests"
}

func validateCustomGates(a AppConfig) error {
	p := a.PromotionPolicy
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	for i, g := range p.CustomGates {
		prefix := fmt.Sprintf("application %q promotion_policy.custom_gates[%d]", a.Name, i)
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return fmt.Errorf("%s: name is required", prefix)
		}
		if seen[name] {
			return fmt.Errorf("%s: duplicate name %q", prefix, name)
		}
		seen[name] = true
		if g.MinApprovals < 0 {
			return fmt.Errorf("%s: min_approvals is negative", prefix)
		}
		for _, r := range g.Rings {
			if !ring.IsValid(r) {
				return fmt.Errorf("%s: unknown ring %q", prefix, r)
			}
		}
		switch g.KindOr() {
		case CustomGateTest:
		case CustomGateJIRA:
			jc := g.JIRA
			if jc == nil && p.ChangeRequest != nil {
				jc = p.ChangeRequest.JIRA
			}
			if jc == nil || strings.TrimSpace(jc.BaseURL) == "" || strings.TrimSpace(jc.Email) == "" {
				return fmt.Errorf("%s: jira.base_url and jira.email are required (or set change_request.jira)", prefix)
			}
		case CustomGateOpsAPI:
			if g.OpsAPI == nil || strings.TrimSpace(g.OpsAPI.BaseURL) == "" {
				return fmt.Errorf("%s: opsapi.base_url is required", prefix)
			}
			if _, err := url.Parse(g.OpsAPI.BaseURL); err != nil {
				return fmt.Errorf("%s: opsapi.base_url: %w", prefix, err)
			}
		case CustomGateMetric:
			if g.Metric == nil {
				return fmt.Errorf("%s: metric block is required", prefix)
			}
			switch g.Metric.OpOr() {
			case MetricLT, MetricLTE, MetricGT, MetricGTE, MetricEQ:
			default:
				return fmt.Errorf("%s: metric.op must be lt, lte, gt, gte or eq", prefix)
			}
			if g.Metric.DemoValue == nil && strings.TrimSpace(g.Metric.Query) == "" {
				return fmt.Errorf("%s: metric.query is required unless demo_value is set", prefix)
			}
		case CustomGateReplicas:
			if g.Replicas == nil {
				return fmt.Errorf("%s: replicas block is required", prefix)
			}
			if g.Replicas.MinReady < 0 {
				return fmt.Errorf("%s: replicas.min_ready is negative", prefix)
			}
		default:
			return fmt.Errorf("%s: unknown kind %q (want jira, opsapi, test, metric or replicas)", prefix, g.Kind)
		}
	}
	return nil
}
