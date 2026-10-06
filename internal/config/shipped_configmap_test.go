package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The ConfigMap under deploy/k8s is applied verbatim by the deploy workflow and
// is the only description of every managed app, so a mistake in it is not a
// mistake in one app: registering a github-deployed app the loader rejects (or
// whose token env is empty) exits the process at startup and takes jobshout,
// beacon and opsapi down with it. That happened on 2026-09-08 when academy was
// enabled without its token, and only maxUnavailable:0 kept the old pod
// serving.
//
// So the shipped file is parsed here, through the same Load the binary uses,
// where a typo costs a red check instead of a control plane.
func TestShippedConfigMapLoads(t *testing.T) {
	// The deployment injects both of these from secret/ring-promoter; the
	// file deliberately holds neither, and Validate refuses a config without
	// them. Standing in for the Secret is the whole of what this test has to
	// fake — everything else it checks is what the file itself says.
	t.Setenv("RP_API_TOKEN", "not-a-real-token")
	t.Setenv("RP_DB_DSN", "postgres://user:pass@localhost:5432/ringpromoter")

	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "k8s", "configmap.yaml"))
	if err != nil {
		t.Fatalf("read the shipped ConfigMap: %v", err)
	}

	var cm struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(raw, &cm); err != nil {
		t.Fatalf("deploy/k8s/configmap.yaml is not valid YAML: %v", err)
	}
	body, ok := cm.Data["config.yaml"]
	if !ok {
		t.Fatal(`deploy/k8s/configmap.yaml has no data["config.yaml"]`)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write the extracted config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the shipped ConfigMap would not load — this is what crash-loops the pod:\n%v", err)
	}
	if len(cfg.Apps) == 0 {
		t.Fatal("the shipped ConfigMap registers no applications")
	}

	// Every app that dispatches a workflow needs a ref that exists. The
	// default is "build", which is a branch wslproxy has and nobody else
	// does, so an app that leaves it blank 404s on its first deploy rather
	// than at boot — the one github-deployer mistake this file cannot catch
	// by loading alone.
	for _, a := range cfg.Apps {
		if a.Deployer == "github" && a.GitHub != nil && a.GitHub.Ref == "" {
			t.Errorf("app %q has no github.ref, so it would dispatch on %q", a.Name, "build")
		}
	}
}
