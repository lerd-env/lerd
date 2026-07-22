package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A self-mount whose host source has disappeared is the one line podman refuses
// to start the container with ("statfs <path>: no such file or directory"), so
// it must be dropped while every other mount is left untouched (#1083).
func TestPruneMissingVolumes(t *testing.T) {
	present := t.TempDir()
	missing := filepath.Join(present, "gone")

	content := strings.Join([]string{
		"[Container]",
		"Volume=%h/.local/share/lerd/nginx/nginx.conf:/etc/nginx/nginx.conf:ro,z",
		"Volume=%h:%h:ro",
		"Volume=" + present + ":" + present + ":rw",
		"Volume=" + missing + ":" + missing + ":rw",
		"Volume=" + missing + ":/etc/nginx/custom.d:ro",
		"Volume=lerd-ssh-agent:/ssh-agent",
		"",
	}, "\n")

	got, removed := PruneMissingVolumes(content)

	if len(removed) != 1 || removed[0] != missing {
		t.Fatalf("removed = %v, want [%s]", removed, missing)
	}
	if strings.Contains(got, "Volume="+missing+":"+missing) {
		t.Errorf("stale self-mount survived:\n%s", got)
	}
	for _, keep := range []string{
		"Volume=%h:%h:ro",
		"Volume=" + present + ":" + present + ":rw",
		"Volume=" + missing + ":/etc/nginx/custom.d:ro",
		"Volume=lerd-ssh-agent:/ssh-agent",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("pruning dropped %q:\n%s", keep, got)
		}
	}
}

// Nothing to prune must leave the content byte-identical, so the caller can use
// equality to decide whether a rewrite is needed at all.
func TestPruneMissingVolumes_noChangeWhenAllPresent(t *testing.T) {
	dir := t.TempDir()
	content := "[Container]\nVolume=%h:%h:ro\nVolume=" + dir + ":" + dir + ":rw\n"

	got, removed := PruneMissingVolumes(content)

	if got != content || removed != nil {
		t.Errorf("PruneMissingVolumes changed a healthy quadlet: removed=%v\n%s", removed, got)
	}
}

// RepairMissingMounts is the start-time preflight: it must sweep every quadlet
// on disk, drop the stale lines, and report which unit lost which path so the
// user learns the project responsible instead of a dead nginx.
func TestRepairMissingMounts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	reloads := 0
	orig := DaemonReloadFn
	DaemonReloadFn = func() error { reloads++; return nil }
	t.Cleanup(func() { DaemonReloadFn = orig })

	site := t.TempDir()
	missing := filepath.Join(site, "Modules", "Accounts")
	reg := config.SiteRegistry{Sites: []config.Site{{Name: "erp", Path: site}}}
	if err := config.SaveSites(&reg); err != nil {
		t.Fatal(err)
	}

	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "[Container]\nVolume=%h:%h:ro\nVolume=" + missing + ":" + missing + ":rw\n"
	healthy := "[Container]\nVolume=%h:%h:ro\nVolume=" + site + ":" + site + ":rw\n"
	// A user's own quadlet in the shared directory carries a stale mount too; it
	// is not lerd's to rewrite, so the sweep must leave it exactly as found.
	foreign := "[Container]\nVolume=" + missing + ":" + missing + ":rw\n"
	for name, content := range map[string]string{
		"lerd-nginx.container":     stale,
		"lerd-php85-fpm.container": stale,
		"lerd-mysql.container":     healthy,
		"backup.container":         foreign,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	repairs := RepairMissingMounts()

	if len(repairs) != 2 {
		t.Fatalf("repairs = %+v, want 2", repairs)
	}
	units := map[string]bool{}
	for _, r := range repairs {
		units[r.Unit] = true
		if r.Path != missing {
			t.Errorf("repair path = %s, want %s", r.Path, missing)
		}
		if r.Site != "erp" {
			t.Errorf("repair site = %q, want erp", r.Site)
		}
	}
	if !units["lerd-nginx"] || !units["lerd-php85-fpm"] {
		t.Errorf("repaired units = %v, want lerd-nginx and lerd-php85-fpm", units)
	}
	for _, name := range []string{"lerd-nginx.container", "lerd-php85-fpm.container"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), missing) {
			t.Errorf("%s still references the missing path:\n%s", name, content)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "lerd-mysql.container"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != healthy {
		t.Errorf("healthy quadlet was rewritten:\n%s", got)
	}
	gotForeign, err := os.ReadFile(filepath.Join(dir, "backup.container"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotForeign) != foreign {
		t.Errorf("foreign quadlet was rewritten:\n%s", gotForeign)
	}
	if reloads != 1 {
		t.Errorf("daemon reloads = %d, want 1", reloads)
	}
}

// A clean install must not touch a single unit file or reload the manager.
func TestRepairMissingMounts_noopWhenNothingStale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	reloads := 0
	orig := DaemonReloadFn
	DaemonReloadFn = func() error { reloads++; return nil }
	t.Cleanup(func() { DaemonReloadFn = orig })

	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lerd-nginx.container"), []byte("[Container]\nVolume=%h:%h:ro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if repairs := RepairMissingMounts(); repairs != nil {
		t.Errorf("repairs = %+v, want none", repairs)
	}
	if reloads != 0 {
		t.Errorf("daemon reloads = %d, want 0", reloads)
	}
}
