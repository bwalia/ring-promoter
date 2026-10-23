package config

import (
	"fmt"
	"time"

	"github.com/example/ring-promoter/internal/ring"
)

// Security scan kinds a report can carry and a policy can require.
const (
	ScanSAST      = "sast"
	ScanSCA       = "sca"
	ScanSecrets   = "secrets"
	ScanContainer = "container"
	ScanIaC       = "iac"
	ScanDAST      = "dast"
	ScanLicense   = "license"
)

// ScanKinds lists every known scan kind.
var ScanKinds = []string{ScanSAST, ScanSCA, ScanSecrets, ScanContainer, ScanIaC, ScanDAST, ScanLicense}

// Finding severities, most severe first.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// Severities lists every severity, most severe first.
var Severities = []string{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}

// GateSecurity is the pipeline gate kind that checks each app's security
// scans against its security policy.
const GateSecurity = "security"

// IsScanKind reports whether k is a known scan kind.
func IsScanKind(k string) bool { return inList(k, ScanKinds) }

// IsSeverity reports whether s is a known severity.
func IsSeverity(s string) bool { return inList(s, Severities) }

func inList(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// SecurityPolicy is the AppSec control a version must satisfy before it may
// enter a guarded ring: the scans that must exist for the exact version and
// the most un-waived findings each severity may have.
type SecurityPolicy struct {
	// Rings are the target rings the promotion gate guards. Empty means the
	// default set (acc, prod).
	Rings []string `yaml:"rings"`
	// Require lists the scan kinds that must have reported for the version.
	// Nil means the default (sast, sca, secrets); an explicit empty list
	// requires none.
	Require []string `yaml:"require"`
	// Max is the most un-waived findings allowed per severity, summed across
	// every scan of the version. Severities left out are unlimited. Nil means
	// the default (no critical, no high).
	Max map[string]int `yaml:"max"`
	// MaxAge, when set, fails a required scan older than this.
	MaxAge Duration `yaml:"max_age"`
}

// DefaultSecurityPolicy is the built-in policy a pipeline security gate uses
// for an app with none configured.
func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{
		Require: []string{ScanSAST, ScanSCA, ScanSecrets},
		Max:     map[string]int{SeverityCritical: 0, SeverityHigh: 0},
	}
}

// Guards reports whether the promotion gate applies to ringName.
func (s *SecurityPolicy) Guards(ringName string) bool {
	return s != nil && ringInSet(ringName, gateRings(s.Rings))
}

// GuardedRings returns the rings the policy guards, with the default applied.
func (s SecurityPolicy) GuardedRings() []string { return gateRings(s.Rings) }

// RequiredKinds returns the required scan kinds, with the default applied.
func (s SecurityPolicy) RequiredKinds() []string {
	if s.Require == nil {
		return DefaultSecurityPolicy().Require
	}
	return s.Require
}

// Limits returns the per-severity maximums, with the default applied.
func (s SecurityPolicy) Limits() map[string]int {
	if s.Max == nil {
		return DefaultSecurityPolicy().Max
	}
	return s.Max
}

// MaxAgeStd returns the scan freshness bound (0 = unbounded).
func (s SecurityPolicy) MaxAgeStd() time.Duration { return s.MaxAge.Std() }

// SecurityPolicyFor returns the app's effective security policy and where it
// came from: "app" (its promotion_policy.security), "default" (top-level
// security_policy), or ok=false when neither is configured.
func (c *Config) SecurityPolicyFor(app string) (pol *SecurityPolicy, source string, ok bool) {
	if a, found := c.App(app); found && a.PromotionPolicy != nil && a.PromotionPolicy.Security != nil {
		return a.PromotionPolicy.Security, "app", true
	}
	if c.SecurityPolicy != nil {
		return c.SecurityPolicy, "default", true
	}
	return nil, "", false
}

// validate checks rings, kinds, severities and limits. where prefixes errors.
func (s *SecurityPolicy) validate(where string) error {
	if s == nil {
		return nil
	}
	for _, r := range s.Rings {
		if !ring.IsValid(r) {
			return fmt.Errorf("%s references unknown ring %q", where, r)
		}
	}
	seen := map[string]bool{}
	for _, k := range s.Require {
		if !IsScanKind(k) {
			return fmt.Errorf("%s requires unknown scan kind %q (want one of %v)", where, k, ScanKinds)
		}
		if seen[k] {
			return fmt.Errorf("%s requires scan kind %q twice", where, k)
		}
		seen[k] = true
	}
	for sev, n := range s.Max {
		if !IsSeverity(sev) {
			return fmt.Errorf("%s max has unknown severity %q (want one of %v)", where, sev, Severities)
		}
		if n < 0 {
			return fmt.Errorf("%s max.%s must not be negative", where, sev)
		}
	}
	if s.MaxAge.Std() < 0 {
		return fmt.Errorf("%s max_age must not be negative", where)
	}
	return nil
}
