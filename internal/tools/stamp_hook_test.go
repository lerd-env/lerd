package tools

import "testing"

// Every tool install, native PHP builds included, ends in WriteStamp, so that is
// where lerd-ui learns its cached update answers are out of date.
func TestWriteStampAnnouncesTheInstall(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := OnStampWritten
	t.Cleanup(func() { OnStampWritten = prev })
	called := 0
	OnStampWritten = func() { called++ }

	if err := WriteStamp("composer", "2.9.0"); err != nil {
		t.Fatalf("WriteStamp: %v", err)
	}
	if called != 1 {
		t.Errorf("hook ran %d times, want 1", called)
	}
}

// A native PHP rebuild can reinstall the version already there, and lerd-ui
// still has to drop its update badge, so every install is announced.
func TestWriteStampAnnouncesAReinstallOfTheSameVersion(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := OnStampWritten
	t.Cleanup(func() { OnStampWritten = prev })
	called := 0
	OnStampWritten = func() { called++ }

	for range 2 {
		if err := WriteStamp("composer", "2.9.0"); err != nil {
			t.Fatalf("WriteStamp: %v", err)
		}
	}
	if called != 2 {
		t.Errorf("hook ran %d times, want 2", called)
	}
}

// Reading mise's version rewrites its stamp on every status build. Announcing
// that as an install made lerd-ui rebuild its status, which read the version
// again, about once a second for as long as a dashboard was open.
func TestInstalledVersionRecordsMiseWithoutAnnouncingAnInstall(t *testing.T) {
	offline(t)
	writeHostMise(t)
	if err := WriteStamp("mise", embeddedManifest().Tools["mise"].Version); err != nil {
		t.Fatal(err)
	}
	orig, prev := probeOutput, OnStampWritten
	t.Cleanup(func() { probeOutput, OnStampWritten = orig, prev })
	probeOutput = func(string, ...string) ([]byte, error) { return []byte("2026.11.14 macos-arm64\n"), nil }
	called := 0
	OnStampWritten = func() { called++ }

	for range 3 {
		if v := InstalledVersion("mise"); v != "2026.11.14" {
			t.Fatalf("InstalledVersion(mise) = %q, want 2026.11.14", v)
		}
	}
	if called != 0 {
		t.Errorf("reading the version announced %d installs, want none", called)
	}
}
