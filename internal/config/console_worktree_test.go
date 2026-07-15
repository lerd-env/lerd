package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A worktree checkout isn't a registered site of its own, so `lerd artisan`
// (via GetConsoleCommand) must inherit the parent site's framework instead of
// failing with "no framework assigned".
func TestGetConsoleCommand_worktreeInheritsParent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	installLaravelVersions(t, map[string][2]string{"12": {"8.2", "8.4"}})
	// installLaravelVersions omits the console field, so add a definition that
	// carries one for the resolver to return.
	body := "name: laravel\nlabel: Laravel\nversion: \"12\"\n" +
		"public_dir: public\nconsole: artisan\n" +
		"php:\n  min: \"8.2\"\n  max: \"8.4\"\n" +
		"detect:\n  - composer: laravel/framework\n"
	if err := os.WriteFile(filepath.Join(StoreFrameworksDir(), "laravel@12.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	sitePath := filepath.Join(tmp, "acme")
	if err := os.MkdirAll(filepath.Join(sitePath, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), "feat-a")
	if err := os.MkdirAll(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	writeComposer(t, checkout, "^12.0")
	writeWorktreeMeta(t, sitePath, "feat-a", checkout)

	if err := AddSite(Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}, Framework: "laravel", PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	got, err := GetConsoleCommand(checkout)
	if err != nil {
		t.Fatalf("GetConsoleCommand(worktree) = error %v, want nil", err)
	}
	if got != "artisan" {
		t.Errorf("console = %q, want %q", got, "artisan")
	}
}
