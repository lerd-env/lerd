package podman

import (
	"os/exec"
	"testing"
)

func TestContainerRunningKnown(t *testing.T) {
	cases := []struct {
		name                   string
		stdout, stderr         string
		code                   int
		wantRunning, wantKnown bool
	}{
		{"running", "true", "", 0, true, true},
		{"stopped", "false", "", 0, false, true},
		{"missing", "", `Error: no such object: "lerd-x"`, 125, false, true},
		{"podman unreachable", "", "Cannot connect to Podman. ssh: handshake failed: EOF", 125, false, false},
	}
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	for _, c := range cases {
		execCommand = func(name string, args ...string) *exec.Cmd {
			return fakeExec(c.stdout, c.stderr, c.code)(name, args...)
		}
		running, known := ContainerRunningKnown("lerd-x")
		if running != c.wantRunning || known != c.wantKnown {
			t.Errorf("%s: got (running=%v, known=%v), want (%v, %v)", c.name, running, known, c.wantRunning, c.wantKnown)
		}
	}
}
