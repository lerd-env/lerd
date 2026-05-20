package serviceops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// runStreaming must write stdout to the dump file and not trip the
// "exec: Stdout already set" error that CombinedOutput raises. This is the
// regression guard for the dump step that broke every service migrate.
func TestRunStreaming_WritesStdoutCapturesStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.sql")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	cmd := exec.Command("sh", "-c", "echo dump-body; echo noise >&2")
	if err := runStreaming(cmd, f); err != nil {
		t.Fatalf("runStreaming: %v", err)
	}
	f.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "dump-body" {
		t.Errorf("dump file = %q, want dump-body", got)
	}
}

func TestRunStreaming_FailureIncludesStderr(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "dump.sql"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	cmd := exec.Command("sh", "-c", "echo boom >&2; exit 3")
	err = runStreaming(cmd, f)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q should carry stderr 'boom'", err.Error())
	}
}

// A bare preset version label must resolve to that version's full preset image,
// not get string-substituted as a literal tag. This is the regression guard for
// `migrate postgres 18` producing the invalid tag postgis/postgis:18.
func TestMigrateTargetImage_ResolvesPresetVersionLabel(t *testing.T) {
	cases := []struct {
		preset  string
		version string
		want    string
	}{
		{"postgres", "18", "docker.io/postgis/postgis:18-3.6-alpine"},
		{"postgres", "16", "docker.io/postgis/postgis:16-3.5-alpine"},
		{"postgres-pgvector", "18", "docker.io/pgvector/pgvector:pg18"},
		{"postgres-pgvector", "17", "docker.io/pgvector/pgvector:pg17"},
		{"mysql", "9.7", "docker.io/library/mysql:9.7"},
		{"mariadb", "11", "docker.io/library/mariadb:11"},
	}
	for _, c := range cases {
		p, err := config.LoadPreset(c.preset)
		if err != nil {
			t.Fatalf("LoadPreset(%q): %v", c.preset, err)
		}
		got, err := migrateTargetImage(p, "irrelevant/current:tag", c.version)
		if err != nil {
			t.Fatalf("%s %s: unexpected error: %v", c.preset, c.version, err)
		}
		if got != c.want {
			t.Errorf("%s %s = %q, want %q", c.preset, c.version, got, c.want)
		}
	}
}

// An argument matching no preset version is substituted verbatim onto the
// current image so exact-tag invocations keep working.
func TestMigrateTargetImage_FallsBackToLiteralTag(t *testing.T) {
	p, err := config.LoadPreset("postgres")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	got, err := migrateTargetImage(p, "docker.io/postgis/postgis:16-3.5-alpine", "16.5-3.5-alpine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "docker.io/postgis/postgis:16.5-3.5-alpine"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMigrateTargetImage_NoPresetSubstitutesTag(t *testing.T) {
	got, err := migrateTargetImage(nil, "docker.io/library/redis:7", "8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "docker.io/library/redis:8"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMigrateTargetImage_NoPresetNoCurrentImageErrors(t *testing.T) {
	if _, err := migrateTargetImage(nil, "", "18"); err == nil {
		t.Fatal("expected error when no preset and no current image")
	}
}

func TestResolveMigrateTarget_EmptyVersionErrors(t *testing.T) {
	if _, err := ResolveMigrateTarget("postgres", "docker.io/postgis/postgis:16-3.5-alpine", "  "); err == nil {
		t.Fatal("expected error for blank version argument")
	}
}
