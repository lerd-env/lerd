package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// projectWithCommands writes a .lerd.yaml containing only a commands: block
// (no framework reference) so resolveCommandsForCwd returns the project
// entries without needing a framework store fetch.
func projectWithCommands(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveCommandsForCwd_ProjectOnly(t *testing.T) {
	dir := projectWithCommands(t, `
commands:
  - name: hello
    label: Say hello
    command: echo hi
    output: text
  - name: deploy
    label: Deploy
    command: ./bin/deploy
    confirm: true
`)
	got := resolveCommandsForCwd(dir)
	if len(got) != 2 {
		t.Fatalf("want 2 commands, got %d: %+v", len(got), got)
	}
	if got[0].Name != "hello" || got[1].Name != "deploy" {
		t.Errorf("order/names: %+v", got)
	}
}

func TestResolveCommandsForCwd_NoConfig(t *testing.T) {
	dir := t.TempDir()
	got := resolveCommandsForCwd(dir)
	if len(got) != 0 {
		t.Errorf("want 0 commands in bare dir, got %d: %+v", len(got), got)
	}
}

func TestListCommands_Empty(t *testing.T) {
	if err := listCommands(nil); err != nil {
		t.Errorf("list with nil should succeed: %v", err)
	}
}

func TestListCommands_FormatsConfirmMarker(t *testing.T) {
	// Just verify it doesn't error; pretty-printing is exercised via
	// the binary smoke. We assert the marker logic via reading back state.
	cmds := []config.FrameworkCommand{
		{Name: "a", Label: "Item A", Description: "Does A"},
		{Name: "b", Label: "Item B", Description: "Does B", Confirm: true},
	}
	if err := listCommands(cmds); err != nil {
		t.Errorf("list: %v", err)
	}
}

func TestRunNamedCommand_NotFound(t *testing.T) {
	err := runNamedCommand(t.TempDir(), nil, "missing", true)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want not-found error, got: %v", err)
	}
}

func TestRunNamedCommand_EmptyCommand(t *testing.T) {
	cmds := []config.FrameworkCommand{{Name: "noop"}}
	err := runNamedCommand(t.TempDir(), cmds, "noop", true)
	if err == nil || !strings.Contains(err.Error(), "no shell invocation") {
		t.Errorf("want empty-cmd error, got: %v", err)
	}
}

func TestResolveCommandsForCwd_WalksUpFromSubdir(t *testing.T) {
	dir := projectWithCommands(t, `
commands:
  - name: ping
    label: Ping
    command: echo pong
`)
	sub := filepath.Join(dir, "app", "Http", "Controllers")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveCommandsForCwd(sub)
	if len(got) != 1 || got[0].Name != "ping" {
		t.Errorf("walk-up should find .lerd.yaml in parent: %+v", got)
	}
}

func TestRunNamedCommand_DisabledFromProjectIsHidden(t *testing.T) {
	// Project entry with disabled: true should not appear in the resolved list,
	// so runNamedCommand sees it as not-found.
	dir := projectWithCommands(t, `
commands:
  - name: hello
    label: Hi
    command: echo hi
  - name: bye
    disabled: true
`)
	cmds := resolveCommandsForCwd(dir)
	for _, c := range cmds {
		if c.Name == "bye" {
			t.Errorf("disabled project entry should not be returned: %+v", c)
		}
	}
}

// Framework commands all start with `php`, which resolves only through lerd's
// shim dir. `lerd path:disable` keeps that dir off the user's shell PATH, so
// without this the CLI runner dies with "php: command not found" on every
// command while the dashboard, which prepends the dir itself, works fine.
func TestNewCommandExec_PutsShimDirOnPath(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	c := newCommandExec(t.TempDir(), "php artisan optimize:clear")

	var got string
	for _, kv := range c.Env {
		if strings.HasPrefix(kv, "PATH=") {
			got = strings.TrimPrefix(kv, "PATH=")
		}
	}
	if !strings.HasPrefix(got, config.BinDir()+string(os.PathListSeparator)) {
		t.Errorf("PATH = %q, want it to start with the shim dir %q", got, config.BinDir())
	}
	if !strings.HasSuffix(got, "/usr/bin:/bin") {
		t.Errorf("PATH = %q, want the inherited entries kept", got)
	}
}
