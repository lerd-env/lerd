package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestCustomWorktreeContainerName(t *testing.T) {
	if got := CustomWorktreeContainerName("nestapp", "feat-login"); got != "lerd-custom-nestapp-feat-login" {
		t.Errorf("CustomWorktreeContainerName = %q, want lerd-custom-nestapp-feat-login", got)
	}
}

func TestGenerateCustomWorktreeContainerQuadlet(t *testing.T) {
	content, err := GenerateCustomWorktreeContainerQuadlet("nestapp", "feat-login", "/home/user/projects/nestapp-feat-login", 3000)
	if err != nil {
		t.Fatalf("GenerateCustomWorktreeContainerQuadlet: %v", err)
	}

	checks := []struct {
		label    string
		contains string
	}{
		{"image", "Image=lerd-custom-nestapp:local"},
		{"container name", "ContainerName=lerd-custom-nestapp-feat-login"},
		{"network", "Network=lerd"},
		{"project volume", "Volume=/home/user/projects/nestapp-feat-login:/home/user/projects/nestapp-feat-login:rw"},
		{"hosts volume", "/etc/hosts:ro,z"},
		{"lerd bin volume", config.BinDir() + ":" + config.BinDir() + ":ro,z"},
		{"security opt", "--security-opt=label=disable"},
		{"workdir", "--workdir=/home/user/projects/nestapp-feat-login"},
		{"site env", "Environment=LERD_SITE=nestapp"},
		{"branch env", "Environment=LERD_BRANCH=feat-login"},
		{"restart", "Restart=always"},
		{"description", "Description=Lerd custom worktree container (nestapp/feat-login)"},
		{"install", "WantedBy=default.target"},
	}

	for _, check := range checks {
		if !strings.Contains(content, check.contains) {
			t.Errorf("%s: quadlet missing %q\n\nFull content:\n%s", check.label, check.contains, content)
		}
	}
	if strings.Contains(content, "PublishPort=") {
		t.Errorf("worktree container must not publish a host port, got:\n%s", content)
	}
}

func TestGenerateCustomWorktreeContainerQuadletRejectsUnmountablePath(t *testing.T) {
	if _, err := GenerateCustomWorktreeContainerQuadlet("nestapp", "feat", "relative/path", 3000); err == nil {
		t.Fatal("expected an error for a path that cannot be bind-mounted")
	}
}

func TestCleanupStaleCustomWorktreeContainers(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	live := CustomWorktreeContainerName("nestapp", "live")
	stale := CustomWorktreeContainerName("nestapp", "stale")
	parent := CustomContainerName("nestapp")
	for _, name := range []string{live, stale, parent} {
		if err := os.WriteFile(filepath.Join(dir, name+".container"), []byte("[Container]\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	prev := execCommand
	execCommand = func(_ string, _ ...string) *exec.Cmd { return exec.Command("true") }
	t.Cleanup(func() { execCommand = prev })
	prevLifecycle := UnitLifecycle
	UnitLifecycle = nil
	t.Cleanup(func() { UnitLifecycle = prevLifecycle })

	CleanupStaleCustomWorktreeContainers("nestapp", []string{"live"})

	if _, err := os.Stat(filepath.Join(dir, live+".container")); err != nil {
		t.Errorf("live worktree quadlet was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, stale+".container")); !os.IsNotExist(err) {
		t.Errorf("stale worktree quadlet still exists (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, parent+".container")); err != nil {
		t.Errorf("parent quadlet was removed: %v", err)
	}
}
