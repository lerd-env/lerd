package cli

import (
	"path/filepath"
	"testing"
)

// storeless isolates a test from the machine's own store and composer home, so
// nothing here reads what the developer running it happens to have installed.
func storeless(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("COMPOSER_HOME", home)
	return home
}

func TestRunGlobalHostCLI_leavesAProjectScriptAlone(t *testing.T) {
	storeless(t)
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{"artisan", "migrate"}, nil); took {
		t.Error("a project's own script must run in the container")
	}
}

func TestRunGlobalHostCLI_leavesAnInvocationWithNoScriptAlone(t *testing.T) {
	storeless(t)
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{"-r", "echo 1;"}, nil); took {
		t.Error("php -r runs no file and must stay in the container")
	}
}

func TestRunGlobalHostCLI_leavesAnUndeclaredGlobalBinaryAlone(t *testing.T) {
	home := storeless(t)
	bin := filepath.Join(home, "vendor", "bin", "psysh")
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{bin}, nil); took {
		t.Error("a global binary no package declares must run in the container")
	}
}
