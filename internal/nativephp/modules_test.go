package nativephp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every module ships under the same file name for every PHP it was built
// against, so a shared directory would leave the last version installed
// deciding which xdebug all the others load.
func TestModulesDirIsPerVersion(t *testing.T) {
	if ModulesDir("8.4") == ModulesDir("8.5") {
		t.Fatalf("8.4 and 8.5 share a modules directory: %s", ModulesDir("8.4"))
	}
	if !strings.Contains(ModulesDir("8.4"), "8.4") {
		t.Errorf("modules dir does not name its version: %s", ModulesDir("8.4"))
	}
}

func TestDevtoolsExtensionPathFindsTheVersionsModule(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := ModulesDir("8.4")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	so := filepath.Join(dir, "lerd_devtools-8.4.so")
	if err := os.WriteFile(so, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DevtoolsExtensionPath("8.4"); got != so {
		t.Errorf("DevtoolsExtensionPath = %q, want %q", got, so)
	}
	// A version with no build of the collector must report nothing rather than
	// a path, or PHP warns on every request for a file that is not there.
	if got := DevtoolsExtensionPath("8.5"); got != "" {
		t.Errorf("DevtoolsExtensionPath(8.5) = %q, want empty", got)
	}
}
