package logcolor

import (
	"strings"
	"testing"
)

// colourOn clears NO_COLOR for the duration of a test so the colour-on
// assertions hold on a machine or CI runner that exports it.
func colourOn(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
}

func TestVars(t *testing.T) {
	colourOn(t)
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
	colourOn(t)
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
	colourOn(t)
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
	colourOn(t)
	out := ShellExports()
	if !strings.Contains(out, "export FORCE_COLOR=1\n") {
		t.Errorf("ShellExports() = %q, want export lines", out)
	}
	if strings.Count(out, "export ") != len(Vars()) {
		t.Errorf("ShellExports() = %q, want one export per var", out)
	}
}

func TestTerminalPassthrough(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    []string
	}{
		{
			name:    "truecolor terminal carries COLORTERM and a 256-colour TERM",
			environ: []string{"TERM=foot", "COLORTERM=truecolor", "PATH=/bin"},
			want:    []string{"COLORTERM=truecolor", "TERM=xterm-256color"},
		},
		{
			name:    "256-colour TERM alone still lifts the pinned xterm",
			environ: []string{"TERM=screen-256color"},
			want:    []string{"TERM=xterm-256color"},
		},
		{
			name:    "a plain terminal is left to podman's own pin",
			environ: []string{"TERM=xterm"},
			want:    nil,
		},
		{
			name:    "NO_COLOR travels alone",
			environ: []string{"TERM=wezterm", "COLORTERM=truecolor", "FORCE_COLOR=1", "NO_COLOR=1"},
			want:    []string{"NO_COLOR=1"},
		},
		{
			name:    "an empty NO_COLOR is not an opt-out",
			environ: []string{"NO_COLOR=", "COLORTERM=24bit"},
			want:    []string{"COLORTERM=24bit", "TERM=xterm-256color"},
		},
		{
			name:    "FORCE_COLOR is forwarded verbatim, zero included",
			environ: []string{"TERM=xterm", "FORCE_COLOR=0"},
			want:    []string{"FORCE_COLOR=0"},
		},
		{
			name:    "an empty FORCE_COLOR is nothing to forward",
			environ: []string{"TERM=xterm", "FORCE_COLOR="},
			want:    nil,
		},
		{
			name:    "a dumb terminal is told nothing about colour",
			environ: []string{"TERM=dumb", "COLORTERM=truecolor"},
			want:    nil,
		},
		{
			name:    "nothing to say about a terminal-less environment",
			environ: []string{"PATH=/bin"},
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TerminalPassthrough(tt.environ)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Errorf("TerminalPassthrough(%v) = %v, want %v", tt.environ, got, tt.want)
			}
		})
	}
}

func TestEnviron(t *testing.T) {
	colourOn(t)
	base := []string{"PATH=/bin"}
	got := Environ(base)
	if len(got) != len(base)+len(Vars()) {
		t.Fatalf("Environ() = %v, want base plus vars", got)
	}
	if got[0] != "PATH=/bin" {
		t.Errorf("Environ() dropped the base environment: %v", got)
	}
}
