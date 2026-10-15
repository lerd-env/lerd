package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestLoad_ReturnsValidAndReportsBroken(t *testing.T) {
	isolate(t)
	writePlugin(t, Dir(), "good", "api_version: 1\nname: good\n")
	writePlugin(t, Dir(), "bad", "api_version: 9\nname: bad\n")
	if err := os.MkdirAll(filepath.Join(Dir(), "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, errs := Load()
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("want only the good plugin, got %+v", got)
	}
	if len(errs) != 1 || errs[0].Dir != filepath.Join(Dir(), "bad") {
		t.Fatalf("want one load error for bad, got %+v", errs)
	}
}

func TestLoad_NoPluginsDir(t *testing.T) {
	isolate(t)
	got, errs := Load()
	if len(got) != 0 || len(errs) != 0 {
		t.Fatalf("a missing plugins dir is an empty set, got %v %v", got, errs)
	}
}

func TestCommand_SetsPluginEnv(t *testing.T) {
	isolate(t)
	dir := writePlugin(t, Dir(), "envy", "api_version: 1\nname: envy\ncommands:\n  - {name: x, run: bin/run}\n")
	script := "#!/bin/sh\necho \"$LERD_PLUGIN_NAME|$LERD_PLUGIN_DIR|$LERD_SURFACE|$LERD_SITE|$1\"\ntest -d \"$LERD_PLUGIN_DATA\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}

	cmd, err := p.Command("bin/run", SurfaceCLI, []string{"arg1"}, map[string]string{"LERD_SITE": "shop"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v (%s)", err, out)
	}
	want := "envy|" + dir + "|cli|shop|arg1"
	if strings.TrimSpace(string(out)) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCommand_RefusesUndeclaredFile(t *testing.T) {
	isolate(t)
	dir := writePlugin(t, Dir(), "p", "api_version: 1\nname: p\n")
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Command("../../../bin/sh", SurfaceCLI, nil, nil); err == nil {
		t.Fatal("a path outside the plugin must be refused")
	}
}
