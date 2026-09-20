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
