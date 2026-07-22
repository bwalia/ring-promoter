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
	// DeployerGitHub triggers an existing GitHub Actions workflow (for apps
	// deployed to VMs by their own CI/CD, e.g. wslproxy) rather than talking
	// to Kubernetes. It requires a per-app `github:` block.
	DeployerGitHub = "github"
	// DeployerK8sJob runs each deploy as a Kubernetes Job executing the app's
	// deployment script (see internal/executor/k8sjob). It requires a per-app
	// `k8sjob:` block.
	DeployerK8sJob = "k8sjob"

	HealthHTTP   = "http"
	HealthAlways = "always"

	StorePostgres = "postgres"
	StoreMemory   = "memory"
)

// Defaults applied when a value is not set.
const (
	defaultRetryCount = 3
	defaultRetryDelay = 5 * time.Second
	defaultOpTimeout  = 10 * time.Minute
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	// ListenAddr is the HTTP bind address, e.g. ":8080".
	ListenAddr string `yaml:"listen_addr"`
	// APIToken guards every /api route (bearer token). Prefer setting this via
	// the RP_API_TOKEN environment variable / Secret rather than the file.
	APIToken string `yaml:"api_token"`
	// ProdPassword, when set, is additionally required for any operation that
	// deploys to the LAST ring (production): promoting into it, seeding it
	// directly, or enabling auto-promote into it. Rollbacks are exempt so
	// incident response is never slowed down. Empty = no extra protection.
	// Prefer the RP_PROD_PASSWORD environment variable / Secret over the file.
	ProdPassword string `yaml:"production_password"`

	// Deployer selects the deploy backend: "kubectl" or "log".
	Deployer string `yaml:"deployer"`
	// Health selects the health backend: "http" or "always".
	Health string `yaml:"health"`

	// OperationTimeout bounds a single seed/promote/rollback end-to-end
	// (deploy + rollout wait + health retries + any auto-rollback). The
	// operation runs detached from the HTTP request, so a client disconnect or
	// load-balancer idle-timeout cannot abort an in-flight deploy or rollback.
	OperationTimeout Duration `yaml:"operation_timeout"`

	Retry    RetryConfig    `yaml:"retry"`
	Database DatabaseConfig `yaml:"database"`
	Ollama   OllamaConfig   `yaml:"ollama"`
	Apps     []AppConfig    `yaml:"apps"`
}

// OllamaConfig configures the optional AI failure-diagnosis feature: when a
// seed/promote/rollback fails, the UI can ask an Ollama server to explain why
// in simple language. The feature is enabled only when both URL and JWTSecret
// are set; otherwise the diagnose endpoint reports it as unavailable.
type OllamaConfig struct {
	// URL is the Ollama server base URL, e.g. "https://ollama.workstation.co.uk".
	URL string `yaml:"url"`
	// Model is the Ollama model used for diagnosis. Default "qwen3-coder:30b".
	Model string `yaml:"model"`
	// JWTSecret signs the per-request HS256 JWT sent in the x-api-key header
	// (the auth gateway in front of Ollama verifies it). Prefer setting this
	// via the RP_OLLAMA_JWT_SECRET environment variable / Secret over the file.
	JWTSecret string `yaml:"jwt_secret"`
}

// Enabled reports whether AI diagnosis is fully configured.
func (o OllamaConfig) Enabled() bool { return o.URL != "" && o.JWTSecret != "" }

// RetryConfig controls the post-deploy health-check retry loop.
//
// Count and Delay are pointers so that an explicit 0 (e.g. `count: 0`, meaning
// "one health check, no retries before rollback") is honored and not confused
// with an unset value that should receive the default.
type RetryConfig struct {
	// Count is the number of additional attempts after the first check.
	// nil = unset (defaulted); 0 = exactly one check, no retries.
	Count *int `yaml:"count"`
	// Delay is the wait between attempts. nil = unset (defaulted).
	Delay *Duration `yaml:"delay"`
}

// RetryCount returns the resolved retry count (default if unset).
func (r RetryConfig) RetryCount() int {
	if r.Count != nil {
		return *r.Count
	}
	return defaultRetryCount
}

// RetryDelay returns the resolved delay between attempts (default if unset).
func (r RetryConfig) RetryDelay() time.Duration {
	if r.Delay != nil {
		return r.Delay.Std()
	}
	return defaultRetryDelay
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
	// DisplayName is the human-friendly title the UI shows for this app (e.g.
	// "diytaxreturn proxy"). Optional; empty means "show the name". Only a
	// label: API paths, stored ring state and history keep using Name, so
	// changing the display name never orphans data.
	DisplayName string `yaml:"display_name"`
	// Deployer selects the deploy mechanism for THIS app, overriding the global
	// `deployer`. One of "kubectl", "log" or "github". Empty means "use the
	// global deployer". This lets one control plane manage Kubernetes apps and
	// VM/CI-deployed apps (e.g. wslproxy) side by side.
	Deployer string `yaml:"deployer"`
	// GitHub configures the CI-dispatch deployer; required when Deployer is
	// "github", ignored otherwise.
	GitHub *GitHubDeployConfig `yaml:"github"`
	// K8sJob configures the Kubernetes Job deployer; required when Deployer is
	// "k8sjob", ignored otherwise.
	K8sJob *K8sJobConfig `yaml:"k8sjob"`
	// PromotionPolicy adds gates (maintenance windows, QA/release sign-off, a
	// valid change-request code) that must be satisfied before a version may be
	// deployed into a sensitive ring. Optional; nil means no extra gating.
	PromotionPolicy *PromotionPolicy `yaml:"promotion_policy"`
	// Rings maps a ring name (see package ring) to its deploy target.
	Rings map[string]RingConfig `yaml:"rings"`
}

// GitHubDeployConfig configures the "github" deployer for one application: it
// triggers a workflow-dispatch on the app's own CI/CD pipeline. The API token
// itself is NOT stored here — it comes from the environment variable named by
// TokenEnv (populated from a Secret).
type GitHubDeployConfig struct {
	// Owner and Repo identify the repository hosting the workflow.
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
	// Workflow is the workflow file name or numeric id to dispatch.
	Workflow string `yaml:"workflow"`
	// Ref is the git ref the workflow runs FROM (branch/tag). Default "build".
	Ref string `yaml:"ref"`
	// VersionAsRef dispatches the workflow ON the deployed version's git ref
	// instead of Ref. For workflows with no version input that build whatever
	// ref they run from (e.g. ios_release.yml: dispatch on a v* tag releases
	// that tag, on main releases main). The workflow file must exist on every
	// deployable ref.
	VersionAsRef bool `yaml:"version_as_ref"`
	// DeployMode is the value sent as the mode input. Default "full".
	DeployMode string `yaml:"deploy_mode"`
	// Input-name overrides for the dispatch payload. They default to the
	// wslproxy deploy-single-environment.yml schema (ENV / DEPLOY_BRANCH /
	// DEPLOY_MODE) but are configurable for other workflows. Set any of them to
	// "-" to OMIT that input entirely — required for workflows that do not
	// declare it (GitHub 422s on undeclared inputs), e.g. spectoncr's
	// deploy-spectoncr.yml has no version or mode input. NOTE: leaving a name
	// blank does NOT omit — a blank name falls back to its default (ENV /
	// DEPLOY_BRANCH / DEPLOY_MODE) and is still sent; only "-" omits.
	EnvInput     string `yaml:"env_input"`
	VersionInput string `yaml:"version_input"`
	ModeInput    string `yaml:"mode_input"`
	// ExtraInputs are additional static dispatch inputs sent verbatim.
	ExtraInputs map[string]string `yaml:"extra_inputs"`
	// TokenEnv names the environment variable holding the API token. Default
	// "RP_GITHUB_TOKEN".
	TokenEnv string `yaml:"token_env"`
	// APIBaseURL overrides the GitHub API base (e.g. for GitHub Enterprise).
	APIBaseURL string `yaml:"api_base_url"`
	// PollInterval is how often the dispatched run is polled. Default 15s.
	PollInterval *Duration `yaml:"poll_interval"`
	// RunLookupTimeout bounds how long to wait for the dispatched run to appear.
	// Default 60s.
	RunLookupTimeout *Duration `yaml:"run_lookup_timeout"`
}

// K8sJobConfig configures the "k8sjob" deployer for one application: each
// seed/promote/rollback runs as a Kubernetes Job whose container executes the
// app's deployment script. Ring Promoter injects the runner contract as
// environment variables (RP_APP, RP_RING, RP_VERSION, RP_TARGET_ENV,
// RP_EXECUTION_ID); the script signals the outcome through its exit code and
// its stdout/stderr become the live deploy log.
type K8sJobConfig struct {
	// Namespace the Jobs run in. Default "ring-exec".
	Namespace string `yaml:"namespace"`
	// Image is the runner image containing the deployment script. Required.
	// Pin a version (no :latest) — a runner change should ship like any other
	// version change.
	Image string `yaml:"image"`
	// Command and Args override the image's entrypoint, e.g.
	// command: ["/scripts/deploy.sh"].
	Command []string `yaml:"command"`
	Args    []string `yaml:"args"`
	// Env are additional environment variables, merged with the injected RP_*
	// contract variables (the contract wins on a name clash).
	Env map[string]string `yaml:"env"`
	// EnvFromSecrets / EnvFromConfigMaps expose whole Secrets/ConfigMaps (in
	// the Job's namespace) to the container as environment. Secret VALUES
	// never appear in this file — only names.
	EnvFromSecrets    []string `yaml:"env_from_secrets"`
	EnvFromConfigMaps []string `yaml:"env_from_config_maps"`
	// ImagePullSecrets name the registry credentials for pulling Image.
	ImagePullSecrets []string `yaml:"image_pull_secrets"`
	// ServiceAccount the Job pods run as. Empty = the namespace's default
	// ServiceAccount; set "ring-deploy-job" (see deploy/k8s/rbac.yaml) when
	// the script needs to talk to the cluster.
	ServiceAccount string          `yaml:"service_account"`
	Resources      K8sJobResources `yaml:"resources"`
	// Timeout bounds one Job end-to-end (activeDeadlineSeconds). Default 30m.
	Timeout *Duration `yaml:"timeout"`
	// Retries is the Job's backoffLimit: extra attempts after a failed one,
	// each in a fresh pod. Default 0.
	Retries *int `yaml:"retries"`
	// TTLAfterFinished keeps the finished Job (and its logs) inspectable
	// before Kubernetes garbage-collects it. Default 1h.
	TTLAfterFinished *Duration          `yaml:"ttl_after_finished"`
	NodeSelector     map[string]string  `yaml:"node_selector"`
	Tolerations      []K8sJobToleration `yaml:"tolerations"`
	// SecurityContext, when set, is applied to the Job container. Leave it unset
	// for ordinary deploy scripts (kubectl/helm) — they need no elevation. Set
	// `privileged: true` only for a runner that builds container images
	// in-cluster (e.g. BuildKit): the daemon needs it. Scope such a runner to a
	// single app and a dedicated ServiceAccount.
	SecurityContext *K8sJobSecurityContext `yaml:"security_context"`
	// Affinity is a raw Kubernetes affinity object passed through verbatim.
	Affinity    map[string]any    `yaml:"affinity"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
	// PollInterval is how often the Job's status is polled. Default 3s.
	PollInterval *Duration `yaml:"poll_interval"`
}

// K8sJobResources are the Job container's compute requests/limits, as
// Kubernetes quantity strings (e.g. "250m", "512Mi").
type K8sJobResources struct {
	CPURequest    string `yaml:"cpu_request"`
	MemoryRequest string `yaml:"memory_request"`
	CPULimit      string `yaml:"cpu_limit"`
	MemoryLimit   string `yaml:"memory_limit"`
}

// K8sJobToleration mirrors a Kubernetes toleration.
type K8sJobToleration struct {
	Key      string `yaml:"key"`
	Operator string `yaml:"operator"`
	Value    string `yaml:"value"`
	Effect   string `yaml:"effect"`
}

// K8sJobSecurityContext is the subset of a container securityContext an app may
// request for its deploy Job. Pointer fields keep an explicit false/0 distinct
// from unset. Only image-building runners should need `privileged`.
type K8sJobSecurityContext struct {
	Privileged             *bool  `yaml:"privileged"`
	RunAsUser              *int64 `yaml:"run_as_user"`
	RunAsGroup             *int64 `yaml:"run_as_group"`
	RunAsNonRoot           *bool  `yaml:"run_as_non_root"`
	ReadOnlyRootFilesystem *bool  `yaml:"read_only_root_filesystem"`
}

// Defaults applied to an unset k8sjob field.
const (
	defaultK8sJobNamespace = "ring-exec"
	defaultK8sJobTimeout   = 30 * time.Minute
	defaultK8sJobTTL       = time.Hour
	defaultK8sJobPoll      = 3 * time.Second
)

// ResolvedNamespace returns the namespace (default if unset).
func (k *K8sJobConfig) ResolvedNamespace() string {
	if k.Namespace != "" {
		return k.Namespace
	}
	return defaultK8sJobNamespace
}

// ResolvedTimeout returns the job timeout (default if unset).
func (k *K8sJobConfig) ResolvedTimeout() time.Duration {
	if k.Timeout != nil {
		return k.Timeout.Std()
	}
	return defaultK8sJobTimeout
}

// ResolvedRetries returns the backoffLimit (default 0; explicit 0 honored).
func (k *K8sJobConfig) ResolvedRetries() int {
	if k.Retries != nil {
		return *k.Retries
	}
	return 0
}

// ResolvedTTL returns the finished-Job retention (default if unset).
func (k *K8sJobConfig) ResolvedTTL() time.Duration {
	if k.TTLAfterFinished != nil {
		return k.TTLAfterFinished.Std()
	}
	return defaultK8sJobTTL
}

// ResolvedPollInterval returns the status poll cadence (default if unset).
func (k *K8sJobConfig) ResolvedPollInterval() time.Duration {
	if k.PollInterval != nil {
		return k.PollInterval.Std()
	}
	return defaultK8sJobPoll
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
	// HealthURL is the URL whose response is checked for ring health.
	HealthURL string `yaml:"health_url"`
	// HealthExpectStatus, when non-zero, is the exact HTTP status code that
	// means healthy for this ring — instead of the default "any 2xx". Use it
	// for endpoints whose healthy response is not 2xx: e.g. spectoncr's auth-
	// gated registry returns 401 on GET /v2/ when it is up (that 401 is exactly
	// the signal deploy-spectoncr.yml itself asserts as healthy), so its rings
	// set health_expect_status: 401.
	HealthExpectStatus int `yaml:"health_expect_status"`
	// HealthVersionField, when set, names the JSON field in the health response
	// body that reports the RUNNING version (dotted path for nested fields,
	// e.g. "version" or "build.version"). Post-deploy health checks then also
	// require the reported version to equal the version just deployed — so an
	// old version still answering "200 OK" no longer passes as a successful
	// deployment. The app must report the exact version string being deployed
	// (the image tag for kubectl apps, the branch/ref for CI-deployed apps).
	// Mutually exclusive with HealthVersionHeader.
	HealthVersionField string `yaml:"health_version_field"`
	// HealthVersionHeader names a response header carrying the running version
	// (e.g. "X-App-Version"), as an alternative to HealthVersionField.
	HealthVersionHeader string `yaml:"health_version_header"`
	// TargetEnv is the environment name a non-Kubernetes deployer ships this
	// ring to (e.g. "int", "test", "prod"). Required for the "github" deployer;
	// ignored by the kubectl deployer.
	TargetEnv string `yaml:"target_env"`
	// Ref pins the version (git branch/tag/sha) this ring always deploys,
	// overriding the seeded/promoted version. Use it when a ring is fixed to a
	// source branch — e.g. acceptance may only ever run `release`: setting
	// `ref: release` makes both seed and promote to that ring ship `release`,
	// so "promote to acc" deploys release while int/test carry main.
	// A pinned ring that also sets HealthVersionField/Header records, after a
	// healthy deploy, the version its endpoint REPORTS running (e.g. v1.0.36)
	// instead of the literal ref name — the pin controls what is dispatched,
	// the endpoint tells us what actually shipped.
	// Only meaningful for branch/CI-based deployers (github); leave empty
	// otherwise (the kubectl deployer treats the version as an image tag).
	Ref string `yaml:"ref"`
	// AutoPromote, when set, makes config the OWNER of this ring's auto-promote
	// switch: the value is reconciled onto stored state at start-up, and the
	// API's toggle refuses to change it (409). Leave it UNSET to keep the
	// historical behaviour — auto-promote is then runtime-only state, off by
	// default, flipped through the API/UI and never touched by config.
	//
	// Absent, true and false are three distinct states, hence the pointer: a
	// plain bool would make every existing config silently claim ownership of
	// every ring and switch auto-promote off wherever an operator had turned it
	// on.
	//
	// Enabling auto-promote INTO the production ring is refused here (see
	// Validate) — that path is guarded by RP_PROD_PASSWORD at the API, and
	// config must not become a way around it.
	AutoPromote *bool `yaml:"auto_promote"`
}

// AutoPromoteOwnedByConfig reports whether config declares this ring's
// auto-promote setting, making it authoritative over stored state.
func (r RingConfig) AutoPromoteOwnedByConfig() bool { return r.AutoPromote != nil }

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
	if v := os.Getenv("RP_PROD_PASSWORD"); v != "" {
		c.ProdPassword = v
	}
	if v := os.Getenv("RP_DEPLOYER"); v != "" {
		c.Deployer = v
	}
	if v := os.Getenv("RP_HEALTH"); v != "" {
		c.Health = v
	}
	if v := os.Getenv("RP_OLLAMA_URL"); v != "" {
		c.Ollama.URL = v
	}
	if v := os.Getenv("RP_OLLAMA_MODEL"); v != "" {
		c.Ollama.Model = v
	}
	if v := os.Getenv("RP_OLLAMA_JWT_SECRET"); v != "" {
		c.Ollama.JWTSecret = v
	}
	if v := os.Getenv("RP_DB_DRIVER"); v != "" {
		c.Database.Driver = v
	}
	if v := os.Getenv("RP_DB_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("RP_RETRY_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Retry.Count = &n
		}
	}
	if v := os.Getenv("RP_RETRY_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			dd := Duration(d)
			c.Retry.Delay = &dd
		}
	}
	if v := os.Getenv("RP_OP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.OperationTimeout = Duration(d)
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
	if c.Retry.Count == nil {
		n := defaultRetryCount
		c.Retry.Count = &n
	}
	if c.Retry.Delay == nil {
		d := Duration(defaultRetryDelay)
		c.Retry.Delay = &d
	}
	if c.OperationTimeout == 0 {
		c.OperationTimeout = Duration(defaultOpTimeout)
	}
	if c.Ollama.Model == "" {
		c.Ollama.Model = "qwen3-coder:30b"
	}
}

// Validate checks the configuration for obvious mistakes.
func (c *Config) Validate() error {
	if c.APIToken == "" {
		return fmt.Errorf("api token is required (set RP_API_TOKEN or api_token)")
	}
	switch c.Deployer {
	case DeployerKubectl, DeployerLog, DeployerGitHub, DeployerK8sJob:
	default:
		return fmt.Errorf("unknown deployer %q (want %q, %q, %q or %q)", c.Deployer, DeployerKubectl, DeployerLog, DeployerGitHub, DeployerK8sJob)
	}
	switch c.Health {
	case HealthHTTP, HealthAlways:
	default:
		return fmt.Errorf("unknown health checker %q (want %q or %q)", c.Health, HealthHTTP, HealthAlways)
	}
	if c.Retry.RetryCount() < 0 {
		return fmt.Errorf("retry count must not be negative")
	}
	if c.Retry.RetryDelay() < 0 {
		return fmt.Errorf("retry delay must not be negative")
	}
	if c.OperationTimeout.Std() <= 0 {
		return fmt.Errorf("operation timeout must be positive")
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
		for rname, rc := range a.Rings {
			if !ring.IsValid(rname) {
				return fmt.Errorf("application %q references unknown ring %q", a.Name, rname)
			}
			if s := rc.HealthExpectStatus; s != 0 && (s < 100 || s > 599) {
				return fmt.Errorf("application %q ring %q has invalid health_expect_status %d (want an HTTP code 100-599, or 0 for any 2xx)", a.Name, rname, s)
			}
			if rc.HealthVersionField != "" && rc.HealthVersionHeader != "" {
				return fmt.Errorf("application %q ring %q sets both health_version_field and health_version_header (choose one)", a.Name, rname)
			}
			if err := validateAutoPromote(a, rname, rc); err != nil {
				return err
			}
		}
		if err := c.validateAppDeployer(a); err != nil {
			return err
		}
		if err := validatePromotionPolicy(a); err != nil {
			return err
		}
	}
	return nil
}

// ProdRingName is the last ring in the pipeline — the one deploys into which
// are guarded by the production password.
func ProdRingName() string {
	all := ring.All()
	return all[len(all)-1].Name
}

// validateAutoPromote checks a ring's config-declared auto-promote setting.
// Only `true` is constrained: it needs somewhere to promote INTO, and it must
// not be the hands-free path into production.
func validateAutoPromote(a AppConfig, rname string, rc RingConfig) error {
	if rc.AutoPromote == nil || !*rc.AutoPromote {
		return nil
	}
	next, ok := ring.Next(rname)
	if !ok {
		return fmt.Errorf("application %q ring %q sets auto_promote: true but %q is the last ring — there is nothing to promote into", a.Name, rname, rname)
	}
	if _, ok := a.Rings[next.Name]; !ok {
		return fmt.Errorf("application %q ring %q sets auto_promote: true but its next ring %q is not configured for this app", a.Name, rname, next.Name)
	}
	// The production guard. Enabling auto-promote into the last ring requires
	// RP_PROD_PASSWORD at the API (see api.handleAutoPromote), precisely so
	// auto-promote cannot be used to sidestep that password. A config-declared
	// value never passes through that handler, so config is not allowed to open
	// that path at all — turn it on through the API, where the password is
	// checked.
	if next.Name == ProdRingName() {
		return fmt.Errorf("application %q ring %q sets auto_promote: true, which would promote into the production ring %q automatically; config must not enable that (it bypasses the production password) — enable it via PUT /api/apps/%s/rings/%s/auto-promote instead", a.Name, rname, next.Name, a.Name, rname)
	}
	return nil
}

// DeployerFor returns the effective deployer kind for an app: its own override
// if set, otherwise the global deployer.
func (c *Config) DeployerFor(a AppConfig) string {
	if a.Deployer != "" {
		return a.Deployer
	}
	return c.Deployer
}

// validateAppDeployer checks an app's deployer selection and any deployer-
// specific requirements (e.g. github needs a github block + per-ring env).
func (c *Config) validateAppDeployer(a AppConfig) error {
	switch a.Deployer {
	case "", DeployerKubectl, DeployerLog, DeployerGitHub, DeployerK8sJob:
	default:
		return fmt.Errorf("application %q has unknown deployer %q", a.Name, a.Deployer)
	}

	switch c.DeployerFor(a) {
	case DeployerGitHub:
		g := a.GitHub
		if g == nil {
			return fmt.Errorf("application %q uses the github deployer but has no `github` block", a.Name)
		}
		if g.Owner == "" || g.Repo == "" || g.Workflow == "" {
			return fmt.Errorf("application %q github block requires owner, repo and workflow", a.Name)
		}
		for rname, rc := range a.Rings {
			if rc.TargetEnv == "" {
				return fmt.Errorf("application %q ring %q needs target_env for the github deployer", a.Name, rname)
			}
		}
	case DeployerK8sJob:
		j := a.K8sJob
		if j == nil {
			return fmt.Errorf("application %q uses the k8sjob deployer but has no `k8sjob` block", a.Name)
		}
		if j.Image == "" {
			return fmt.Errorf("application %q k8sjob block requires an image", a.Name)
		}
		if j.ResolvedRetries() < 0 {
			return fmt.Errorf("application %q k8sjob retries must not be negative", a.Name)
		}
		if j.ResolvedTimeout() <= 0 {
			return fmt.Errorf("application %q k8sjob timeout must be positive", a.Name)
		}
	}
	return nil
}

// TokenEnvName returns the environment variable holding the API token.
func (g *GitHubDeployConfig) TokenEnvName() string {
	if g.TokenEnv != "" {
		return g.TokenEnv
	}
	return "RP_GITHUB_TOKEN"
}
