package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// writeProject lays down a .lerd.yaml so the picked-service side of the check
// has something to read.
func writeProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .lerd.yaml: %v", err)
	}
	return dir
}

func TestMissingDeclaredServices_coversWhatTheProjectPicksNotJustWhatTheFrameworkRequires(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-mysql": true}, map[string]string{"lerd-mysql": "active"})
	dir := writeProject(t, "services:\n  - mysql\n  - redis\n")

	got := MissingDeclaredServices(dir, &config.Framework{Requires: []string{"opensearch"}})

	want := []string{"opensearch", "redis"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("missing = %v, want %v: a service the project picks counts as declared", got, want)
	}
}

func TestMissingDeclaredServices_ignoresSQLiteAndInstalledServices(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-mysql": true}, map[string]string{"lerd-mysql": "active"})
	dir := writeProject(t, "services:\n  - mysql\n  - sqlite\n")

	if got := MissingDeclaredServices(dir, &config.Framework{}); len(got) != 0 {
		t.Errorf("missing = %v, want none: mysql is installed and sqlite is a file, not a service", got)
	}
}

func TestRequiredServices_offersToInstallWhatIsMissing(t *testing.T) {
	withStubs(t, nil, nil)
	dir := writeProject(t, "services:\n  - redis\n")

	c, ok := checkRequiredServices(dir, &config.Framework{Name: "magento", Requires: []string{"opensearch"}})
	if !ok {
		t.Fatal("a site declaring services should produce a check")
	}
	if c.Status != StatusFail {
		t.Errorf("status = %q, want %q", c.Status, StatusFail)
	}
	if c.Fix != FixInstallServices {
		t.Errorf("fix = %q, want %q so the dashboard can install them", c.Fix, FixInstallServices)
	}
	for _, name := range []string{"opensearch", "redis"} {
		if !strings.Contains(c.Detail, name) {
			t.Errorf("detail %q does not name the missing service %q", c.Detail, name)
		}
	}
}

func TestRequiredServices_installedButStoppedIsStillJustAWarning(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-redis": true}, map[string]string{"lerd-redis": "inactive"})
	dir := writeProject(t, "services:\n  - redis\n")

	c, ok := checkRequiredServices(dir, &config.Framework{})
	if !ok {
		t.Fatal("a site picking a service should produce a check")
	}
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want %q: it is installed, only stopped", c.Status, StatusWarn)
	}
	if c.Fix == FixInstallServices {
		t.Error("a stopped service should not be offered an install")
	}
}

func TestStoppedDeclaredServices_namesWhatIsInstalledButNotRunning(t *testing.T) {
	withStubs(t,
		map[string]bool{"lerd-mysql": true, "lerd-phpmyadmin": true},
		map[string]string{"lerd-mysql": "active", "lerd-phpmyadmin": "inactive"})
	dir := writeProject(t, "services:\n  - mysql\n  - phpmyadmin\n")

	got := StoppedDeclaredServices(dir, &config.Framework{})

	if len(got) != 1 || got[0] != "phpmyadmin" {
		t.Errorf("stopped = %v, want only phpmyadmin: mysql is running", got)
	}
}

func TestRequiredServices_offersToStartWhatIsStopped(t *testing.T) {
	withStubs(t, map[string]bool{"lerd-phpmyadmin": true}, map[string]string{"lerd-phpmyadmin": "inactive"})
	dir := writeProject(t, "services:\n  - phpmyadmin\n")

	c, ok := checkRequiredServices(dir, &config.Framework{})
	if !ok {
		t.Fatal("a site picking a service should produce a check")
	}
	if c.Fix != FixStartServices {
		t.Errorf("fix = %q, want %q so the dashboard can start it rather than print a command", c.Fix, FixStartServices)
	}
}
