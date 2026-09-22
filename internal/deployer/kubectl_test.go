package deployer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// fakeKubectl writes a stand-in kubectl that appends its arguments to a log
// file, and returns a deployer using it plus the log path.
func fakeKubectl(t *testing.T) (*KubectlDeployer, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\necho \"$@\" >> '" + logPath + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := NewKubectlDeployer(nil, 45*time.Second)
	d.bin = bin
	return d, logPath
}

func kubectlCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

var kubectlTarget = Target{App: "web", Ring: "int", Namespace: "ns-int", Deployment: "web", Container: "web", Image: "repo/web"}

func TestKubectlRestart_RolloutRestartThenStatus(t *testing.T) {
	want := []string{
		"-n ns-int rollout restart deployment/web",
		"-n ns-int rollout status deployment/web --timeout=45s",
	}
	for name, req := range map[string]RestartRequest{
		"all targets":    {Version: "v1"},
		"named (subset)": {Version: "v1", Deployments: []string{"web", "web"}},
	} {
		t.Run(name, func(t *testing.T) {
			d, logPath := fakeKubectl(t)
			if err := d.Restart(context.Background(), kubectlTarget, req); err != nil {
				t.Fatalf("restart: %v", err)
			}
			got := kubectlCalls(t, logPath)
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("kubectl calls = %q, want %q", got, want)
			}
		})
	}
}

// Named Deployments must be a subset of the ring's configured target: kubectl
// never restarts something Ring Promoter does not manage.
func TestKubectlRestart_RejectsDeploymentOutsideTargets(t *testing.T) {
	d, logPath := fakeKubectl(t)
	req := RestartRequest{Version: "v1", Deployments: []string{"web", "redis"}}
	if err := d.ValidateRestart(kubectlTarget, req); !errors.Is(err, ErrInvalidDeployments) {
		t.Fatalf("ValidateRestart: expected ErrInvalidDeployments, got %v", err)
	}
	if err := d.Restart(context.Background(), kubectlTarget, req); !errors.Is(err, ErrInvalidDeployments) {
		t.Fatalf("Restart: expected ErrInvalidDeployments, got %v", err)
	}
	if calls := kubectlCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("kubectl must not run for an invalid request, got %q", calls)
	}
}

func TestValidateDeploymentNames(t *testing.T) {
	tooMany := make([]string, MaxRestartDeployments+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("d%d", i)
	}
	for _, tc := range []struct {
		names []string
		ok    bool
	}{
		{nil, true},
		{[]string{"jobshout-api", "jobshout-web", "a", "a1-b2"}, true},
		{tooMany[:MaxRestartDeployments], true},
		{tooMany, false},
		{[]string{"Jobshout"}, false},
		{[]string{"-api"}, false},
		{[]string{"api-"}, false},
		{[]string{"a.b"}, false},
		{[]string{""}, false},
		{[]string{"api; rm -rf /"}, false},
		{[]string{strings.Repeat("a", 64)}, false},
	} {
		err := ValidateDeploymentNames(tc.names)
		if tc.ok && err != nil {
			t.Errorf("%q: unexpected error %v", tc.names, err)
		}
		if !tc.ok && !errors.Is(err, ErrInvalidDeployments) {
			t.Errorf("%q: expected ErrInvalidDeployments, got %v", tc.names, err)
		}
	}
}

// Without a restart spec an execution backend refuses a restart.
func TestExecDeployerRestart_UnsupportedWithoutRestartSpec(t *testing.T) {
	d := FromExecutor(nil, nil, nil, 0)
	if err := d.ValidateRestart(Target{App: "web", Ring: "int"}, RestartRequest{}); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("ValidateRestart: expected ErrRestartUnsupported, got %v", err)
	}
	if err := d.Restart(context.Background(), Target{App: "web", Ring: "int"}, RestartRequest{}); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("Restart: expected ErrRestartUnsupported, got %v", err)
	}
}

// With a restart spec the restart runs as an ordinary execution of it.
func TestExecDeployerRestart_RunsRestartSpec(t *testing.T) {
	fx := &fakeExecutor{ex: &fakeExecution{statuses: []executor.Status{{Phase: executor.PhaseSucceeded}}}}
	d := newAdapter(fx).WithRestartSpec(func(t Target, req RestartRequest) (executor.Spec, error) {
		return executor.Spec{App: t.App, Ring: t.Ring, Args: []string{"restart"}, Env: map[string]string{
			executor.EnvVersion:            req.Version,
			executor.EnvRestartDeployments: strings.Join(req.Deployments, " "),
		}}, nil
	})
	req := RestartRequest{Version: "v7", Deployments: []string{"api", "web"}}
	if err := d.Restart(context.Background(), Target{App: "web", Ring: "int"}, req); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if fx.spec.Args[0] != "restart" || fx.spec.Env[executor.EnvVersion] != "v7" ||
		fx.spec.Env[executor.EnvRestartDeployments] != "api web" {
		t.Fatalf("restart ran the wrong spec: %+v", fx.spec)
	}
	// Deployment names are shape-checked even though the task owns their meaning.
	bad := RestartRequest{Version: "v7", Deployments: []string{"Bad Name"}}
	if err := d.ValidateRestart(Target{App: "web", Ring: "int"}, bad); !errors.Is(err, ErrInvalidDeployments) {
		t.Fatalf("expected ErrInvalidDeployments, got %v", err)
	}
}
