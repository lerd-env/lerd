package node

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagerByName_Mise(t *testing.T) {
	if got := ManagerByName("mise").Name(); got != "mise" {
		t.Errorf("ManagerByName(\"mise\").Name() = %q, want \"mise\"", got)
	}
}

// mise is a binary of its own like fnm, so lerd's node/npm/npx wrappers are
// what put it on PATH for host workers.
func TestWritesPathShims_Mise(t *testing.T) {
	if !WritesPathShims(miseManager{}) {
		t.Error("mise should write PATH shims")
	}
}

// A mise the user already installed is the one lerd drives, so a version they
// manage themselves is not shadowed by a second copy.
func TestFindMise_PrefersUserInstall(t *testing.T) {
	home := t.TempDir()
	userBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(userBin, 0o755); err != nil {
		t.Fatal(err)
	}
	userMise := filepath.Join(userBin, "mise")
	if err := os.WriteFile(userMise, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	noLookPath := func(string) (string, error) { return "", os.ErrNotExist }
	if got := findMise(home, noLookPath); got != userMise {
		t.Errorf("findMise = %q, want the user's own install at %q", got, userMise)
	}
}

// Nothing installed anywhere means no mise to drive, and Available must say so
// rather than handing back a path that cannot run.
func TestFindMise_EmptyWhenAbsent(t *testing.T) {
	home := t.TempDir()
	noLookPath := func(string) (string, error) { return "", os.ErrNotExist }
	if got := findMise(home, noLookPath); got != "" {
		t.Errorf("findMise = %q, want empty when mise is not installed", got)
	}
}

// PATH is the fallback for a mise installed somewhere lerd does not probe,
// such as a package manager's own prefix.
func TestFindMise_FallsBackToPath(t *testing.T) {
	home := t.TempDir()
	onPath := func(string) (string, error) { return "/opt/somewhere/bin/mise", nil }
	if got := findMise(home, onPath); got != "/opt/somewhere/bin/mise" {
		t.Errorf("findMise = %q, want the PATH hit", got)
	}
}

// MiseInstallPath is where lerd puts mise when the user has none. It has to be
// mise's own canonical location, not lerd's private bin dir, or the user ends
// up with a second mise their shell never sees.
func TestMiseInstallPath_IsCanonical(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".local", "bin", "mise")
	if got := miseInstallPath(home); got != want {
		t.Errorf("miseInstallPath = %q, want %q", got, want)
	}
}

func TestParseMiseList(t *testing.T) {
	raw := `[
	  {"version":"22.11.0","install_path":"/x","installed":true,"active":false},
	  {"version":"20.18.0","install_path":"/y","installed":true,"active":false},
	  {"version":"22.9.0","install_path":"/z","installed":true,"active":false}
	]`
	if got, want := parseMiseListFull(raw), []string{"22.11.0", "20.18.0", "22.9.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseMiseListFull = %v, want %v", got, want)
	}
	if got, want := dedupeMajors(parseMiseListFull(raw)), []string{"22", "20"}; !reflect.DeepEqual(got, want) {
		t.Errorf("majors = %v, want %v", got, want)
	}
}

// An uninstalled version mise merely knows about is not one lerd can run.
func TestParseMiseList_SkipsNotInstalled(t *testing.T) {
	raw := `[{"version":"22.11.0","installed":false,"active":false}]`
	if got := parseMiseListFull(raw); len(got) != 0 {
		t.Errorf("parseMiseListFull = %v, want nothing for an uninstalled version", got)
	}
}

func TestParseMiseList_EmptyAndGarbage(t *testing.T) {
	if got := parseMiseListFull("[]"); len(got) != 0 {
		t.Errorf("parseMiseListFull(\"[]\") = %v, want empty", got)
	}
	if got := parseMiseListFull("not json"); len(got) != 0 {
		t.Errorf("parseMiseListFull(garbage) = %v, want empty", got)
	}
}

// The prefix is spliced into a worker unit's `sh -c` body, so the tool selector
// carries the version and the whole thing survives a path with a space.
func TestMiseExecPrefix(t *testing.T) {
	m := miseManager{bin: "/opt/my tools/mise"}
	got := m.ExecPrefix("22")
	if !strings.Contains(got, "node@22") {
		t.Errorf("ExecPrefix = %q, want the node@22 selector", got)
	}
	if !strings.HasPrefix(got, "'/opt/my tools/mise'") {
		t.Errorf("ExecPrefix = %q, want the binary path quoted", got)
	}
	if !strings.HasSuffix(got, "--") {
		t.Errorf("ExecPrefix = %q, want it to end with the -- separator", got)
	}
}

// An unset or unusable version runs the version mise has configured rather than
// inventing a selector, matching what the other managers do with "default".
func TestMiseExecPrefix_NoVersion(t *testing.T) {
	m := miseManager{bin: "/usr/bin/mise"}
	if got := m.ExecPrefix(""); strings.Contains(got, "node@") {
		t.Errorf("ExecPrefix(\"\") = %q, want no pinned selector", got)
	}
}
