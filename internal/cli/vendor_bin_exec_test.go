package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// The exec routes hand podman their own PATH, which replaces the container's,
// so the opt-in bun has to be in the one they build or a bare `bun` resolves
// for a request and not for `lerd php` or a console command.
func TestContainerExecEnvArgs_KeepsBunOnPath(t *testing.T) {
	args := strings.Join(containerExecEnvArgs("/home/u/site"), " ")
	want := "PATH=/home/u/site/vendor/bin:" + podman.ContainerPath + ":"
	if !strings.Contains(args, want) {
		t.Errorf("env args = %q, want a PATH built from %q", args, want)
	}
}

// ssh in the container ignores HOME and would only read root's known_hosts,
// so every exec route has to hand it the user's own file.
func TestContainerExecEnvArgs_PointsSSHAtTheUserKnownHosts(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	args := strings.Join(containerExecEnvArgs("/home/u/site"), " ")
	want := "GIT_SSH_COMMAND=ssh -o UserKnownHostsFile='/home/u/.ssh/known_hosts'"
	if !strings.Contains(args, want) {
		t.Errorf("env args = %q, want %q", args, want)
	}
}
