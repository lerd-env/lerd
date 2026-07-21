package logcolor

import (
	"strings"
	"testing"
)

func TestVars(t *testing.T) {
	got := strings.Join(Vars(), " ")
	for _, want := range []string{"FORCE_COLOR=1", "CLICOLOR_FORCE=1", "TERM=xterm-256color"} {
		if !strings.Contains(got, want) {
			t.Errorf("Vars() = %q, missing %q", got, want)
		}
	}
}

func TestNoColorWins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if len(Vars()) != 0 {
		t.Errorf("Vars() = %v, want empty when NO_COLOR is set", Vars())
	}
	if got := QuadletEnvLines(); got != "" {
		t.Errorf("QuadletEnvLines() = %q, want empty", got)
	}
	if got := ShellExports(); got != "" {
		t.Errorf("ShellExports() = %q, want empty", got)
	}
	if got := PodmanExecArgs(); len(got) != 0 {
		t.Errorf("PodmanExecArgs() = %v, want empty", got)
	}
}

func TestPodmanExecArgs(t *testing.T) {
	args := PodmanExecArgs()
	if len(args) != len(Vars()) {
		t.Fatalf("PodmanExecArgs() = %v, want one flag per var", args)
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "--env=") {
			t.Errorf("arg %q missing --env= prefix", a)
		}
	}
}

func TestQuadletEnvLines(t *testing.T) {
	out := QuadletEnvLines()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("QuadletEnvLines() = %q, want trailing newline", out)
	}
	if !strings.Contains(out, `Environment="FORCE_COLOR=1"`) {
		t.Errorf("QuadletEnvLines() = %q, want quoted Environment line", out)
	}
	if strings.Count(out, "Environment=") != len(Vars()) {
		t.Errorf("QuadletEnvLines() = %q, want one line per var", out)
	}
}

func TestShellExports(t *testing.T) {
	out := ShellExports()
	if !strings.Contains(out, "export FORCE_COLOR=1\n") {
		t.Errorf("ShellExports() = %q, want export lines", out)
	}
	if strings.Count(out, "export ") != len(Vars()) {
		t.Errorf("ShellExports() = %q, want one export per var", out)
	}
}

func TestEnviron(t *testing.T) {
	base := []string{"PATH=/bin"}
	got := Environ(base)
	if len(got) != len(base)+len(Vars()) {
		t.Fatalf("Environ() = %v, want base plus vars", got)
	}
	if got[0] != "PATH=/bin" {
		t.Errorf("Environ() dropped the base environment: %v", got)
	}
}
