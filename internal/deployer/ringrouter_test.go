package deployer

import (
	"context"
	"testing"
)

type recDeployer struct {
	name  string
	calls *[]string
}

func (d recDeployer) Deploy(_ context.Context, t Target, v string) error {
	*d.calls = append(*d.calls, d.name+":"+t.Ring+":"+v)
	return nil
}
func (d recDeployer) Restart(_ context.Context, t Target, _ RestartRequest) error {
	*d.calls = append(*d.calls, d.name+":restart:"+t.Ring)
	return nil
}
func (d recDeployer) ValidateRestart(Target, RestartRequest) error { return nil }

type vsDeployer struct{ recDeployer }

func (vsDeployer) ListVersions(context.Context) ([]Version, error) {
	return []Version{{Name: "main", Type: "branch"}}, nil
}
func (vsDeployer) ValidateVersion(context.Context, string) error { return ErrVersionNotFound }

func TestRingRouterRoutesByRing(t *testing.T) {
	var calls []string
	def := recDeployer{"github", &calls}
	job := recDeployer{"k8sjob", &calls}
	r := RingRouter(def, map[string]Deployer{"int": job})
	ctx := context.Background()
	for _, ring := range []string{"int", "test", "prod"} {
		if err := r.Deploy(ctx, Target{App: "a", Ring: ring}, "v1"); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.Restart(ctx, Target{Ring: "int"}, RestartRequest{})
	want := []string{"k8sjob:int:v1", "github:test:v1", "github:prod:v1", "k8sjob:restart:int"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestRingRouterVersionSourceFollowsDefault(t *testing.T) {
	var calls []string
	if _, ok := RingRouter(recDeployer{"log", &calls}, nil).(VersionSource); ok {
		t.Fatal("router advertises VersionSource although the default deployer has none")
	}
	r := RingRouter(vsDeployer{recDeployer{"github", &calls}}, map[string]Deployer{"int": recDeployer{"k8sjob", &calls}})
	vs, ok := r.(VersionSource)
	if !ok {
		t.Fatal("router hides the default deployer's VersionSource")
	}
	if err := vs.ValidateVersion(context.Background(), "x"); err != ErrVersionNotFound {
		t.Fatalf("ValidateVersion = %v, want ErrVersionNotFound from the default", err)
	}
}
