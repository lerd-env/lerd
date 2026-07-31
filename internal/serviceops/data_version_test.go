package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// presetTempHome sets up an isolated HOME with a stubbed daemon-reload so a
// quadlet write in a test never touches the developer's real install.
func presetTempHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
}

// writeDataVersion seeds a service data dir holding a cluster written by the
// given version, the way a surviving data dir looks after the config is lost.
func writeDataVersion(t *testing.T, service, marker, content string) {
	t.Helper()
	dir := config.DataSubDir(service)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, marker), []byte(content+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", marker, err)
	}
}

func readQuadletImage(t *testing.T, unit string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(config.QuadletDir(), unit+".container"))
	if err != nil {
		t.Fatalf("read quadlet: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, "Image="); ok {
			return strings.TrimSpace(after)
		}
	}
	t.Fatalf("no Image= line in %s quadlet:\n%s", unit, string(data))
	return ""
}

// A cluster on disk outlives the quadlet and the config entry that described
// it. When only the data dir survives, the version it was written by has to
// drive the image, otherwise the preset's canonical default (postgres 16) is
// mounted over newer files and the container refuses to start.
func TestEnsureDefaultPresetQuadlet_pinsVersionFromSurvivingDataDir(t *testing.T) {
	presetTempHome(t)
	writeDataVersion(t, "postgres", "PG_VERSION", "18")

	if err := EnsureDefaultPresetQuadlet("postgres"); err != nil {
		t.Fatalf("EnsureDefaultPresetQuadlet: %v", err)
	}

	if got := readQuadletImage(t, "lerd-postgres"); !strings.Contains(got, ":18-") {
		t.Errorf("quadlet must run an 18 image over an 18 data dir, got %q", got)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if got := cfg.Services["postgres"].CanonicalVersion; got != "18" {
		t.Errorf("CanonicalVersion = %q, want 18", got)
	}
}

// The reported failure: a stale quadlet naming the older image is the very
// thing InstalledImage reads back, so the preserve-on-disk-image branch keeps
// rewriting 16 over an 18 cluster and even a corrected canonical_version loses.
func TestEnsureDefaultPresetQuadlet_dataDirBeatsStaleQuadletImage(t *testing.T) {
	presetTempHome(t)
	writeDataVersion(t, "postgres", "PG_VERSION", "18")

	if err := os.MkdirAll(config.QuadletDir(), 0o755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	stale := "[Container]\nImage=docker.io/postgis/postgis:16-3.5-alpine\n"
	if err := os.WriteFile(filepath.Join(config.QuadletDir(), "lerd-postgres.container"), []byte(stale), 0o644); err != nil {
		t.Fatalf("seed stale quadlet: %v", err)
	}

	if err := EnsureDefaultPresetQuadlet("postgres"); err != nil {
		t.Fatalf("EnsureDefaultPresetQuadlet: %v", err)
	}

	if got := readQuadletImage(t, "lerd-postgres"); !strings.Contains(got, ":18-") {
		t.Errorf("stale 16 quadlet must not survive an 18 data dir, got %q", got)
	}
}

// Reinstall captures the pre-remove image and pins it back. That must still
// yield to the data dir, otherwise reinstalling a wedged service re-wedges it.
func TestEnsureDefaultPresetQuadletPinned_dataDirBeatsReinstallPin(t *testing.T) {
	presetTempHome(t)
	writeDataVersion(t, "postgres", "PG_VERSION", "18")

	if err := EnsureDefaultPresetQuadletPinned("postgres", "docker.io/postgis/postgis:16-3.5-alpine"); err != nil {
		t.Fatalf("EnsureDefaultPresetQuadletPinned: %v", err)
	}

	if got := readQuadletImage(t, "lerd-postgres"); !strings.Contains(got, ":18-") {
		t.Errorf("reinstall pin must yield to an 18 data dir, got %q", got)
	}
}

// An explicit image in config.yaml is a deliberate human statement, so it stays
// authoritative even when it disagrees with the data dir. Reporting that
// mismatch belongs to doctor, not to a silent rewrite behind the user's back.
func TestEnsureDefaultPresetQuadlet_explicitImagePinWinsOverDataDir(t *testing.T) {
	presetTempHome(t)
	writeDataVersion(t, "postgres", "PG_VERSION", "18")

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	entry := cfg.Services["postgres"]
	entry.Image = "docker.io/postgis/postgis:16-3.5-alpine"
	cfg.Services["postgres"] = entry
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	if err := EnsureDefaultPresetQuadlet("postgres"); err != nil {
		t.Fatalf("EnsureDefaultPresetQuadlet: %v", err)
	}

	// Assert the tag, not the whole ref: on darwin the platform override swaps
	// postgis/postgis for imresamu/postgis and templates the same tag through,
	// so the repo legitimately differs per host while the pinned version cannot.
	if got := readQuadletImage(t, "lerd-postgres"); !strings.HasSuffix(got, ":16-3.5-alpine") {
		t.Errorf("explicit image pin must be honored, got %q", got)
	}
}

// A data dir written by a version the preset does not offer, and a marker file
// that is absent entirely, both have to leave resolution exactly as it was.
func TestEnsureDefaultPresetQuadlet_unknownAndMissingMarkerKeepCanonical(t *testing.T) {
	t.Run("unknown version", func(t *testing.T) {
		presetTempHome(t)
		writeDataVersion(t, "postgres", "PG_VERSION", "9.6")

		if err := EnsureDefaultPresetQuadlet("postgres"); err != nil {
			t.Fatalf("EnsureDefaultPresetQuadlet: %v", err)
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			t.Fatalf("LoadGlobal: %v", err)
		}
		if got := cfg.Services["postgres"].CanonicalVersion; got != "16" {
			t.Errorf("CanonicalVersion = %q, want the canonical 16", got)
		}
	})

	t.Run("no data dir", func(t *testing.T) {
		presetTempHome(t)

		if err := EnsureDefaultPresetQuadlet("postgres"); err != nil {
			t.Fatalf("EnsureDefaultPresetQuadlet: %v", err)
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			t.Fatalf("LoadGlobal: %v", err)
		}
		if got := cfg.Services["postgres"].CanonicalVersion; got != "16" {
			t.Errorf("CanonicalVersion = %q, want the canonical 16", got)
		}
	})
}

func TestMatchVersionByTagString(t *testing.T) {
	postgres := []config.PresetVersion{{Tag: "18"}, {Tag: "17"}, {Tag: "16"}}
	mariadb := []config.PresetVersion{{Tag: "11"}, {Tag: "11.4"}, {Tag: "11.8"}}

	cases := []struct {
		name     string
		content  string
		versions []config.PresetVersion
		want     string
	}{
		{"postgres PG_VERSION", "18", postgres, "18"},
		{"mariadb upgrade info", "11.8.8-MariaDB", mariadb, "11.8"},
		{"longest tag wins", "11.4.5-MariaDB", mariadb, "11.4"},
		{"unknown version", "9.6", postgres, ""},
		{"empty", "", postgres, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchVersionByTagString(tc.content, tc.versions); got != tc.want {
				t.Errorf("matchVersionByTagString(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestPostgresPresetDeclaresDataVersionMarker(t *testing.T) {
	p, err := config.LoadPreset("postgres")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	if p.DataVersionFile != "PG_VERSION" {
		t.Errorf("postgres preset data_version_file = %q, want PG_VERSION", p.DataVersionFile)
	}
	if p, err := config.LoadPreset("mysql"); err != nil {
		t.Fatalf("LoadPreset mysql: %v", err)
	} else if p.DataVersionFile != "" {
		t.Errorf("mysql has no readable marker in its data dir, got %q", p.DataVersionFile)
	}
}
