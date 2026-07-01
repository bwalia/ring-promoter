package deployer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// KubectlDeployer updates an application's Deployment on k3s by shelling out to
// kubectl. When running in-cluster it authenticates automatically with the
// pod's ServiceAccount token; the accompanying RBAC grants permission to patch
// Deployments in the ring namespaces.
//
// kubectl is used (rather than embedding client-go) deliberately: it keeps the
// binary and its dependency tree small while giving battle-tested rollout
// semantics via `kubectl rollout status`.
type KubectlDeployer struct {
	log     *slog.Logger
	bin     string        // kubectl binary, defaults to "kubectl"
	rollout time.Duration // rollout status timeout
}

// NewKubectlDeployer returns a KubectlDeployer. rolloutTimeout bounds how long
// Deploy waits for the rollout to become available.
func NewKubectlDeployer(log *slog.Logger, rolloutTimeout time.Duration) *KubectlDeployer {
	if log == nil {
		log = slog.Default()
	}
	if rolloutTimeout <= 0 {
		rolloutTimeout = 2 * time.Minute
	}
	return &KubectlDeployer{log: log, bin: "kubectl", rollout: rolloutTimeout}
}

// Deploy implements Deployer: it sets the container image and waits for the
// rollout to complete.
func (d *KubectlDeployer) Deploy(ctx context.Context, t Target, version string) error {
	image := t.Image + ":" + version
	d.log.Info("kubectl deploy",
		"app", t.App, "ring", t.Ring, "namespace", t.Namespace,
		"deployment", t.Deployment, "image", image)

	// Set the image tag on the target container.
	if _, err := d.run(ctx, 30*time.Second,
		"-n", t.Namespace, "set", "image",
		"deployment/"+t.Deployment, t.Container+"="+image,
	); err != nil {
		return fmt.Errorf("set image: %w", err)
	}

	// Wait for the new ReplicaSet to become available.
	if _, err := d.run(ctx, d.rollout,
		"-n", t.Namespace, "rollout", "status",
		"deployment/"+t.Deployment,
		fmt.Sprintf("--timeout=%s", d.rollout),
	); err != nil {
		return fmt.Errorf("rollout status: %w", err)
	}
	return nil
}

// LiveVersion implements LiveVersioner by reading the running image tag.
func (d *KubectlDeployer) LiveVersion(ctx context.Context, t Target) (string, error) {
	jsonpath := fmt.Sprintf(
		`jsonpath={.spec.template.spec.containers[?(@.name=="%s")].image}`, t.Container)
	out, err := d.run(ctx, 15*time.Second,
		"-n", t.Namespace, "get", "deployment", t.Deployment, "-o", jsonpath)
	if err != nil {
		return "", err
	}
	image := strings.TrimSpace(out)
	if i := strings.LastIndex(image, ":"); i >= 0 {
		return image[i+1:], nil
	}
	return "", nil
}

// run executes kubectl with the given args under a timeout, returning stdout.
func (d *KubectlDeployer) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, d.bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
