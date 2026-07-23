package config

import "testing"

// verify_version defaults to true: every app that predates the flag must keep
// validating its version, so adding the flag cannot silently loosen anything.
func TestVerifiesVersion_DefaultsToTrue(t *testing.T) {
	for name, g := range map[string]*GitHubDeployConfig{
		"nil config":    nil,
		"unset flag":    {Owner: "bwalia", Repo: "wslproxy"},
		"explicit true": {VerifyVersion: boolPtr(true)},
	} {
		if !g.VerifiesVersion() {
			t.Errorf("%s: VerifiesVersion() = false, want true", name)
		}
	}
}

func TestVerifiesVersion_ExplicitFalse(t *testing.T) {
	g := &GitHubDeployConfig{VerifyVersion: boolPtr(false)}
	if g.VerifiesVersion() {
		t.Fatal("VerifiesVersion() = true for verify_version: false")
	}
}

func boolPtr(b bool) *bool { return &b }
