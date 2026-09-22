package deployer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeKubectl writes a stand-in kubectl that appends its arguments to a log
// file, and returns a deployer using it plus the log path.
func fakeKubectl(t *testing.T) (*KubectlDeployer, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := NewKubectlDeployer(nil, 45*time.Second)
	d.bin = bin
	return d, logPath
}

func TestKubectlRestart_RolloutRestartThenStatus(t *testing.T) {
	d, logPath := fakeKubectl(t)
	tgt := Target{App: "web", Ring: "int", Namespace: "ns-int", Deployment: "web", Container: "web", Image: "repo/web"}
	if err := d.Restart(context.Background(), tgt); err != nil {
		t.Fatalf("restart: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"-n ns-int rollout restart deployment/web",
		"-n ns-int rollout status deployment/web --timeout=45s",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("kubectl calls = %q, want %q", got, want)
	}
}

// An execution backend can only re-run the deploy, so it refuses a restart.
func TestExecDeployerRestart_Unsupported(t *testing.T) {
	d := FromExecutor(nil, nil, nil, 0)
	if err := d.Restart(context.Background(), Target{App: "web", Ring: "int"}); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("expected ErrRestartUnsupported, got %v", err)
	}
}
