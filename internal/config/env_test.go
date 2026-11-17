package config

import "testing"

func TestEnvironmentAliasesPreserveUndermynd(t *testing.T) {
	t.Setenv("UNDERMYND_WORK", "/data/existing-work")
	t.Setenv("UNDERMYND_PUBLIC_ADDR", ":18080")
	if got := LoadEnv(); got.WorkDir != "/data/existing-work" || got.PublicAddr != ":18080" {
		t.Fatal(got)
	}
	t.Setenv("OVERMYND_WORK", "/data/new-work")
	t.Setenv("OVERMYND_PUBLIC_ADDR", ":28080")
	if got := LoadEnv(); got.WorkDir != "/data/new-work" || got.PublicAddr != ":28080" {
		t.Fatal(got)
	}
	t.Setenv("OVERMYND_WORK", "")
	if got := LoadEnv(); got.WorkDir != "" {
		t.Fatal("explicit empty override ignored", got)
	}
}
