package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// An editor installed only as an app bundle, with its shell command never
// added to PATH, still opens a folder: launchd gives lerd-ui a bare PATH, so
// this is the usual case on macOS rather than an edge.
func TestDirCommandOpensAnAppBundleOffPath(t *testing.T) {
	isolate(t)
	t.Setenv("PATH", t.TempDir())
	apps := t.TempDir()
	prev := appBundleDirs
	t.Cleanup(func() { appBundleDirs = prev })
	appBundleDirs = func() []string { return []string{apps} }
	app := filepath.Join(apps, "Cursor.app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEditorConfig(t, "cursor")
	want := []string{"open", "-a", app, "/a"}
	if got := DirCommand("/a"); !reflect.DeepEqual(got, want) {
		t.Fatalf("DirCommand(cursor) = %v, want %v", got, want)
	}
}
