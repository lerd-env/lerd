package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstall_FromDirectory(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "checkout")
	if err := Scaffold("hello", src); err != nil {
		t.Fatal(err)
	}

	p, err := Install(src)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p.Dir != filepath.Join(Dir(), "hello") {
		t.Errorf("installed under %s, want the manifest name", p.Dir)
	}
	if _, err := Install(src); err == nil {
		t.Error("installing the same plugin twice must fail")
	}
}

func TestInstall_InvalidLeavesNothing(t *testing.T) {
	isolate(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ManifestFile), []byte("api_version: 99\nname: broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(src); err == nil {
		t.Fatal("an invalid plugin must be refused")
	}
	entries, _ := os.ReadDir(Dir())
	if len(entries) != 0 {
		t.Errorf("a refused install must leave the plugins dir empty, found %v", entries)
	}
}

// A plugin that no longer loads still has to be removable, or the only way out
// of a broken install would be the file manager.
func TestRemove_WorksForBrokenPluginAndKeepsData(t *testing.T) {
	isolate(t)
	writePlugin(t, Dir(), "broken", "api_version: 99\nname: broken\n")
	if err := os.MkdirAll(DataDirFor("broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Remove("broken"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "broken")); !os.IsNotExist(err) {
		t.Error("plugin folder should be gone")
	}
	if _, err := os.Stat(DataDirFor("broken")); err != nil {
		t.Error("the plugin's data must be kept")
	}
}

func TestRemove_Refusals(t *testing.T) {
	isolate(t)
	for _, name := range []string{"missing", "../etc", ""} {
		if err := Remove(name); err == nil {
			t.Errorf("Remove(%q) should fail", name)
		}
	}
}
