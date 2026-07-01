// Package config loads the Ring Promoter configuration from a YAML file and
// applies environment-variable overrides. The application registry lives in the
// file (mounted from a ConfigMap in k3s) so that adding an application needs no
// code change; secrets and per-environment knobs come from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/example/ring-promoter/internal/ring"
	"gopkg.in/yaml.v3"
)

// Deployer / Health / Store selector values.
const (
	DeployerKubectl = "kubectl"
	DeployerLog     = "log"

	HealthHTTP   = "http"
	HealthAlways = "always"

	StorePostgres = "postgres"
	StoreMemory   = "memory"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	// ListenAddr is the HTTP bind address, e.g. ":8080".
	ListenAddr string `yaml:"listen_addr"`
	// APIToken guards every /api route (bearer token). Prefer setting this via
	// the RP_API_TOKEN environment variable / Secret rather than the file.
	APIToken string `yaml:"api_token"`

	// Deployer selects the deploy backend: "kubectl" or "log".
	Deployer string `yaml:"deployer"`
	// Health selects the health backend: "http" or "always".
	Health string `yaml:"health"`

	Retry    RetryConfig    `yaml:"retry"`
	Database DatabaseConfig `yaml:"database"`
	Apps     []AppConfig    `yaml:"apps"`
}

// RetryConfig controls the post-deploy health-check retry loop.
type RetryConfig struct {
	// Count is the number of additional attempts after the first check.
	Count int `yaml:"count"`
	// Delay is the wait between attempts.
	Delay Duration `yaml:"delay"`
}

// DatabaseConfig selects and configures the persistence backend.
type DatabaseConfig struct {
	// Driver is "postgres" or "memory".
	Driver string `yaml:"driver"`
	// DSN is the Postgres connection string (ignored for the memory driver).
	DSN string `yaml:"dsn"`
}

// AppConfig defines one managed application and how it is deployed / checked in
// each ring.
type AppConfig struct {
	Name string `yaml:"name"`
	// Rings maps a ring name (see package ring) to its deploy target.
	Rings map[string]RingConfig `yaml:"rings"`
}

// RingConfig describes how to deploy and health-check one application in one
// ring.
type RingConfig struct {
	// Namespace is the k8s namespace hosting this ring.
	Namespace string `yaml:"namespace"`
	// Deployment is the name of the k8s Deployment to update.
	Deployment string `yaml:"deployment"`
	// Container is the container within the Deployment whose image is set.
	Container string `yaml:"container"`
	// Image is the image repository (without tag); the tag is the version.
	Image string `yaml:"image"`
	// HealthURL is the URL whose 2xx response means the ring is healthy.
	HealthURL string `yaml:"health_url"`
}

// Duration is a time.Duration that unmarshals from a YAML string like "5s".
type Duration time.Duration

// UnmarshalYAML parses a Go duration string.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// Std returns the value as a time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// App returns the configuration for the named application.
func (c *Config) App(name string) (AppConfig, bool) {
	for _, a := range c.Apps {
		if a.Name == name {
			return a, true
		}
	}
	return AppConfig{}, false
}

// Load reads the YAML file at path (if non-empty), applies environment
// overrides, fills defaults, and validates the result.
func Load(path string) (*Config, error) {
	cfg := &Config{}

	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	cfg.applyEnv()
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv overrides file values with environment variables when set.
func (c *Config) applyEnv() {
	if v := os.Getenv("RP_LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	}
	if v := os.Getenv("RP_API_TOKEN"); v != "" {
		c.APIToken = v
	}
	if v := os.Getenv("RP_DEPLOYER"); v != "" {
		c.Deployer = v
	}
	if v := os.Getenv("RP_HEALTH"); v != "" {
		c.Health = v
	}
	if v := os.Getenv("RP_DB_DRIVER"); v != "" {
		c.Database.Driver = v
	}
	if v := os.Getenv("RP_DB_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("RP_RETRY_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Retry.Count = n
		}
	}
	if v := os.Getenv("RP_RETRY_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Retry.Delay = Duration(d)
		}
	}
}

// applyDefaults fills unset fields with sensible local-development defaults.
func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.Deployer == "" {
		c.Deployer = DeployerLog
	}
	if c.Health == "" {
		c.Health = HealthAlways
	}
	if c.Database.Driver == "" {
		c.Database.Driver = StoreMemory
	}
	if c.Retry.Count == 0 {
		c.Retry.Count = 3
	}
	if c.Retry.Delay == 0 {
		c.Retry.Delay = Duration(5 * time.Second)
	}
}

// Validate checks the configuration for obvious mistakes.
func (c *Config) Validate() error {
	if c.APIToken == "" {
		return fmt.Errorf("api token is required (set RP_API_TOKEN or api_token)")
	}
	switch c.Deployer {
	case DeployerKubectl, DeployerLog:
	default:
		return fmt.Errorf("unknown deployer %q (want %q or %q)", c.Deployer, DeployerKubectl, DeployerLog)
	}
	switch c.Health {
	case HealthHTTP, HealthAlways:
	default:
		return fmt.Errorf("unknown health checker %q (want %q or %q)", c.Health, HealthHTTP, HealthAlways)
	}
	switch c.Database.Driver {
	case StoreMemory:
	case StorePostgres:
		if c.Database.DSN == "" {
			return fmt.Errorf("database dsn is required for the postgres driver")
		}
	default:
		return fmt.Errorf("unknown database driver %q", c.Database.Driver)
	}
	if len(c.Apps) == 0 {
		return fmt.Errorf("no applications configured")
	}

	seen := map[string]bool{}
	for _, a := range c.Apps {
		if a.Name == "" {
			return fmt.Errorf("application with empty name")
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate application %q", a.Name)
		}
		seen[a.Name] = true
		if len(a.Rings) == 0 {
			return fmt.Errorf("application %q has no rings configured", a.Name)
		}
		for rname := range a.Rings {
			if !ring.IsValid(rname) {
				return fmt.Errorf("application %q references unknown ring %q", a.Name, rname)
			}
		}
	}
	return nil
}
