package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// storeSandbox points the framework store and config at temp dirs.
func storeSandbox(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	store := filepath.Join(data, "lerd", "frameworks")
	if err := os.MkdirAll(store, 0755); err != nil {
		t.Fatal(err)
	}
	return store
}

func projectWithEmbeddedDef(t *testing.T, label string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "framework: acme\nframework_def:\n  name: acme\n  label: " + label + "\n  public_dir: public\n"
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A project carrying its own framework definition installs it when the machine
// has none, so cloning such a repo brings the framework along.
func TestDetectFrameworkForDir_installsAnEmbeddedDefinitionWhenAbsent(t *testing.T) {
	store := storeSandbox(t)
	dir := projectWithEmbeddedDef(t, "Acme")

	name, ok := DetectFrameworkForDir(dir)
	if !ok || name != "acme" {
		t.Fatalf("DetectFrameworkForDir = (%q, %v), want (acme, true)", name, ok)
	}
	if _, err := os.Stat(filepath.Join(store, "acme.yaml")); err != nil {
		t.Errorf("the embedded definition was not installed: %v", err)
	}
}

// A store-published framework belongs to every project on the machine, so one
// project's embedded copy must not replace it. This used to happen on a mere
// detection call: the copy overwrote the store entry, and when it omitted the
// detect rules the store carried, detection broke for every project of that
// framework. Differences against a published definition are the link flow's
// conflict prompt to resolve.
func TestDetectFrameworkForDir_doesNotOverwriteAStorePublishedDefinition(t *testing.T) {
	store := storeSandbox(t)
	installed := "name: acme\nlabel: Acme Proper\npublic_dir: public\ndetect:\n    - file: acme.json\n"
	if err := os.WriteFile(filepath.Join(store, "acme.yaml"), []byte(installed), 0644); err != nil {
		t.Fatal(err)
	}
	// The store index is what makes the name store-owned rather than the
	// project's to define.
	index := `{"frameworks":[{"name":"acme","label":"Acme Proper","versions":["1"],"latest":"1","detect":[{"file":"acme.json"}]}]}`
	if err := os.WriteFile(filepath.Join(store, "index.json"), []byte(index), 0644); err != nil {
		t.Fatal(err)
	}
	dir := projectWithEmbeddedDef(t, "Hijacked")

	if _, ok := DetectFrameworkForDir(dir); !ok {
		t.Fatal("DetectFrameworkForDir did not resolve the installed definition")
	}

	after, err := os.ReadFile(filepath.Join(store, "acme.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != installed {
		t.Errorf("a published definition was rewritten by a project's copy:\n%s", after)
	}
}

// A framework the store does not publish belongs to the projects that use it, so
// an edit to the committed definition still propagates.
func TestDetectFrameworkForDir_propagatesAnEditToAProjectOwnedDefinition(t *testing.T) {
	store := storeSandbox(t)
	stale := "name: acme\nlabel: Stale\npublic_dir: public\ndetect:\n    - file: acme.json\n"
	if err := os.WriteFile(filepath.Join(store, "acme.yaml"), []byte(stale), 0644); err != nil {
		t.Fatal(err)
	}
	dir := projectWithEmbeddedDef(t, "Edited")

	if _, ok := DetectFrameworkForDir(dir); !ok {
		t.Fatal("DetectFrameworkForDir did not resolve the definition")
	}

	after, err := os.ReadFile(filepath.Join(store, "acme.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Edited") {
		t.Errorf("an edit to a project-owned definition should propagate:\n%s", after)
	}
}
