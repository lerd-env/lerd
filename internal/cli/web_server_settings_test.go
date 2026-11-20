package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/webcustom"
)

func runWebServer(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewWebServerCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// fakeEditor makes $EDITOR overwrite the file it is handed with body.
func fakeEditor(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	content := filepath.Join(dir, "content")
	os.WriteFile(content, []byte(body), 0o644)
	script := filepath.Join(dir, "ed")
	os.WriteFile(script, []byte("#!/bin/sh\ncat "+content+" > \"$1\"\n"), 0o755)
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", script)
}

// The global settings of lerd's own web server are shown, edited and reset
// from the CLI, a bad edit refused with the reason rather than saved.
func TestWebServerSettings_globalShowEditReset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, err := runWebServer(t, "show", "--global")
	if err != nil || !strings.Contains(out, "every site") {
		t.Fatalf("show: %q %v", out, err)
	}
	if out, _ := runWebServer(t, "show", "--global", "--path"); strings.TrimSpace(out) != webcustom.GlobalPath() {
		t.Errorf("--path printed %q", out)
	}

	fakeEditor(t, "headers:\n  X-A: b\n")
	if out, err := runWebServer(t, "edit", "--global"); err != nil {
		t.Fatalf("edit: %q %v", out, err)
	}
	if c, _ := webcustom.For("x.test"); c.Headers["X-A"] != "b" {
		t.Errorf("edit not saved: %+v", c)
	}

	fakeEditor(t, "nope: 1\n")
	if out, err := runWebServer(t, "edit", "--global"); err == nil || !strings.Contains(out, "global.yaml") {
		t.Fatalf("bad edit: %q %v", out, err)
	}
	if c, _ := webcustom.For("x.test"); c.Headers["X-A"] != "b" {
		t.Error("the bad edit replaced the good file")
	}

	if _, err := runWebServer(t, "reset", "--global"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webcustom.GlobalPath()); !os.IsNotExist(err) {
		t.Error("reset left the file")
	}
}

// While lerd's own web server serves, nginx is gone, so lerd nginx refuses and
// points at the command that edits what the running server reads.
func TestNginxCmd_refusesWhileLerdServes(t *testing.T) {
	stubWebServerSwitch(t, false)
	if err := switchWebServer("go", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cmd := NewNginxCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"show", "--path"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "lerd web-server") {
		t.Fatalf("lerd nginx ran under lerd's own server: %v %q", err, out.String())
	}
}
