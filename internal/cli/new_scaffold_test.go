package cli

import (
	"path/filepath"
	"testing"
)

func TestScaffoldPlan(t *testing.T) {
	target := "/home/me/projects/myapp"

	cases := []struct {
		name        string
		create      string
		extraArgs   []string
		wantPHP     bool
		wantBin     string
		wantTailEnd []string
	}{
		{
			// Every create command in the framework store starts with composer,
			// and composer ships inside lerd rather than on the host.
			name:        "composer runs through the bundled phar",
			create:      "composer create-project --no-install laravel/laravel",
			wantPHP:     true,
			wantTailEnd: []string{"create-project", "--no-install", "laravel/laravel", target},
		},
		{
			name:        "extra args are handed on after the target",
			create:      "composer create-project symfony/skeleton",
			extraArgs:   []string{"--no-interaction"},
			wantPHP:     true,
			wantTailEnd: []string{"create-project", "symfony/skeleton", target, "--no-interaction"},
		},
		{
			// A framework free to declare something composer cannot do still
			// reaches the host binary.
			name:        "a non-composer create command stays on the host",
			create:      "npx create-something",
			wantPHP:     false,
			wantBin:     "npx",
			wantTailEnd: []string{"create-something", target},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := scaffoldPlan(c.create, target, c.extraArgs)
			if plan.inContainer != c.wantPHP {
				t.Fatalf("inContainer = %v, want %v", plan.inContainer, c.wantPHP)
			}
			if c.wantPHP {
				if filepath.Base(plan.args[0]) != "composer.phar" {
					t.Errorf("args[0] = %q, want the bundled composer.phar", plan.args[0])
				}
				assertTail(t, plan.args[1:], c.wantTailEnd)
				return
			}
			if plan.args[0] != c.wantBin {
				t.Errorf("args[0] = %q, want %q", plan.args[0], c.wantBin)
			}
			assertTail(t, plan.args[1:], c.wantTailEnd)
		})
	}
}

func assertTail(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestScaffoldPlan_emptyCreateCommand(t *testing.T) {
	if plan := scaffoldPlan("", "/tmp/x", nil); len(plan.args) != 0 {
		t.Errorf("args = %v, want none for an empty create command", plan.args)
	}
}
