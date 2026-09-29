package linker

import (
	"os"
	"path/filepath"
	"testing"
)

// A version pinned only in the untracked local file belongs to this machine, so
// the committed .php-version must keep the project's own answer.
func TestPinProjectPHPVersion_leavesTheCommittedFileAloneForALocalPin(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".lerd.yaml", "php_version: \"8.4\"\n")
	write(".lerd.local.yaml", "php_version: \"8.5\"\n")
	write(".php-version", "8.4\n")

	pinProjectPHPVersion(dir, "8.5")

	got, _ := os.ReadFile(filepath.Join(dir, ".php-version"))
	if string(got) != "8.4\n" {
		t.Errorf(".php-version = %q, want the committed 8.4 kept", got)
	}
}

func TestPinProjectPHPVersion_pinsWithoutALocalOverride(t *testing.T) {
	dir := t.TempDir()
	pinProjectPHPVersion(dir, "8.5")

	got, _ := os.ReadFile(filepath.Join(dir, ".php-version"))
	if string(got) != "8.5\n" {
		t.Errorf(".php-version = %q, want 8.5", got)
	}
}
