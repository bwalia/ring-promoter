package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/executor/k8sjob"
)

func k8sJobTestConfig() *config.K8sJobConfig {
	timeout := config.Duration(20 * time.Minute)
	ttl := config.Duration(time.Hour)
	return &config.K8sJobConfig{
		Namespace:        "ring-exec",
		Image:            "docker.io/dtzar/helm-kubectl:3.15.4",
		ServiceAccount:   "ring-deploy-job",
		EnvFromSecrets:   []string{"jobshout-registry-creds"},
		HostNetwork:      true,
		Command:          []string{"/bin/sh", "-c"},
		Args:             []string{"helm upgrade ..."},
		RestartArgs:      []string{"kubectl rollout restart ..."},
		Env:              map[string]string{"EXTRA": "1"},
		Resources:        config.K8sJobResources{CPURequest: "100m", MemoryLimit: "512Mi"},
		Timeout:          &timeout,
		TTLAfterFinished: &ttl,
	}
}

// The restart Job is the deploy Job for the current version — same image, SA,
// env_from_secrets, host_network, resources, timeout and ttl — running the
// restart args with the restart half of the runner contract.
func TestK8sJobRestartSpec_ReusesDeployJobWithRestartArgs(t *testing.T) {
	j := k8sJobTestConfig()
	specFor := k8sJobSpec(j)
	tgt := deployer.Target{App: "jobshout", Ring: "int", TargetEnv: "int"}

	deploy, err := specFor(tgt, "v9")
	if err != nil {
		t.Fatal(err)
	}
	restart, err := k8sJobRestartSpec(j, specFor)(tgt, deployer.RestartRequest{
		Version: "v9", Deployments: []string{"jobshout-api", "jobshout-web"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(restart.Command, []string{"/bin/sh", "-c"}) ||
		!reflect.DeepEqual(restart.Args, []string{"kubectl rollout restart ..."}) {
		t.Fatalf("restart command/args = %q %q", restart.Command, restart.Args)
	}
	for k, want := range map[string]string{
		executor.EnvTargetEnv:          "int",
		executor.EnvVersion:            "v9",
		executor.EnvAction:             "restart",
		executor.EnvRestartDeployments: "jobshout-api jobshout-web",
		executor.EnvApp:                "jobshout",
		"EXTRA":                        "1",
	} {
		if restart.Env[k] != want {
			t.Errorf("env %s = %q, want %q", k, restart.Env[k], want)
		}
	}
	// The deploy spec is untouched (no shared env map) ...
	if _, ok := deploy.Env[executor.EnvAction]; ok {
		t.Fatal("restart env leaked into the deploy spec")
	}
	// ... and everything but command/args/env matches it.
	restart.Command, restart.Args, restart.Env = deploy.Command, deploy.Args, deploy.Env
	if !reflect.DeepEqual(restart, deploy) {
		t.Fatalf("restart Job differs from the deploy Job:\nrestart %+v\ndeploy  %+v", restart, deploy)
	}

	// No named deployments → empty RP_RESTART_DEPLOYMENTS (the script's default).
	all, _ := k8sJobRestartSpec(j, specFor)(tgt, deployer.RestartRequest{Version: "v9"})
	if v, ok := all.Env[executor.EnvRestartDeployments]; !ok || v != "" {
		t.Fatalf("RP_RESTART_DEPLOYMENTS = %q (present %t), want empty", v, ok)
	}

	// restart_command overrides the deploy command.
	j.RestartCommand = []string{"/scripts/restart.sh"}
	cmd, _ := k8sJobRestartSpec(j, specFor)(tgt, deployer.RestartRequest{Version: "v9"})
	if strings.Join(cmd.Command, " ") != "/scripts/restart.sh" {
		t.Fatalf("restart command = %q", cmd.Command)
	}
}

// Restart on a k8sjob app is supported only when restart_args is configured.
func TestBuildK8sJobDeployer_RestartNeedsRestartArgs(t *testing.T) {
	ex := k8sjob.New(nil, k8sjob.Options{})
	tgt := deployer.Target{App: "jobshout", Ring: "int"}

	j := k8sJobTestConfig()
	with := buildK8sJobDeployer(config.AppConfig{Name: "jobshout", K8sJob: j}, nil, ex)
	if err := with.ValidateRestart(tgt, deployer.RestartRequest{Version: "v1", Deployments: []string{"jobshout-api"}}); err != nil {
		t.Fatalf("with restart_args: %v", err)
	}

	j2 := k8sJobTestConfig()
	j2.RestartArgs = nil
	without := buildK8sJobDeployer(config.AppConfig{Name: "jobshout", K8sJob: j2}, nil, ex)
	if err := without.ValidateRestart(tgt, deployer.RestartRequest{Version: "v1"}); !errors.Is(err, deployer.ErrRestartUnsupported) {
		t.Fatalf("without restart_args: expected ErrRestartUnsupported, got %v", err)
	}
}
