package deployer

import (
	"context"
	"testing"
)

// fakeVersionSource is a Deployer that also enumerates/validates versions —
// the shape GitHubActionsDeployer has.
type fakeVersionSource struct {
	deployed string
}

func (f *fakeVersionSource) Deploy(_ context.Context, _ Target, version string) error {
	f.deployed = version
	return nil
}

func (f *fakeVersionSource) ListVersions(context.Context) ([]Version, error) {
	return []Version{{Name: "main", Type: "branch"}}, nil
}

func (f *fakeVersionSource) ValidateVersion(context.Context, string) error {
	// Whatever this returns must become unreachable once wrapped.
	panic("ValidateVersion must not be consulted on a wrapped deployer")
}

func TestWithoutVersionSource_HidesTheCapability(t *testing.T) {
	inner := &fakeVersionSource{}

	if _, ok := Deployer(inner).(VersionSource); !ok {
		t.Fatal("precondition: the unwrapped deployer should implement VersionSource")
	}

	wrapped := WithoutVersionSource(inner)
	if _, ok := wrapped.(VersionSource); ok {
		t.Fatal("wrapped deployer still advertises VersionSource, so seeds would " +
			"still be validated against a repository the version does not live in")
	}
}

// The wrapper must hide the capability WITHOUT changing how a deploy runs — it
// is a visibility change, not a behaviour change.
func TestWithoutVersionSource_StillDeploys(t *testing.T) {
	inner := &fakeVersionSource{}
	wrapped := WithoutVersionSource(inner)

	if err := wrapped.Deploy(context.Background(), Target{App: "opsapi", Ring: "int"}, "v1.4.0-12-gabc1234-457"); err != nil {
		t.Fatalf("Deploy through the wrapper: %v", err)
	}
	if inner.deployed != "v1.4.0-12-gabc1234-457" {
		t.Fatalf("inner deployer got version %q, want the version passed through untouched", inner.deployed)
	}
}
