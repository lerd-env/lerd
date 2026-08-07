package config

import (
	"os"
	"path/filepath"
	"testing"
)

func installVersionedDef(t *testing.T, name, version string) {
	t.Helper()
	dir := StoreFrameworksDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "name: " + name + "\nlabel: " + name + "\nversion: \"" + version + "\"\npublic_dir: public\n" +
		"php:\n  min: \"8.1\"\n  max: \"8.3\"\n"
	if err := os.WriteFile(filepath.Join(dir, name+"@"+version+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A project past the newest published definition is served that definition, and
// used to report itself as its version: a WordPress 7 site read "WordPress 6".
// The version it runs is the project's, whichever definition lerd had to borrow.
func TestGetFrameworkForDir_reportsTheVersionAProjectRuns(t *testing.T) {
	setConfigDir(t)
	installVersionedDef(t, "acme", "6")

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: acme\nframework_version: \"7\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok {
		t.Fatal("GetFrameworkForDir resolved nothing")
	}
	if fw.Version != "6" {
		t.Errorf("Version = %q, want the borrowed definition's 6", fw.Version)
	}
	if fw.DetectedVersion != "7" {
		t.Errorf("DetectedVersion = %q, want the project's own 7", fw.DetectedVersion)
	}
	// The borrowed definition is the newest there is, so its PHP range is the
	// best answer available and still applies.
	if fw.VersionGuessed {
		t.Error("VersionGuessed = true, want false: the PHP range still holds for a future version")
	}
}

// The legacy case keeps both: the project's version to report, and the flag
// that says the borrowed definition's PHP range must not constrain it.
func TestGetFrameworkForDir_legacyProjectKeepsItsVersionAndItsRelaxedRange(t *testing.T) {
	setConfigDir(t)
	installVersionedDef(t, "acme", "10")
	installVersionedDef(t, "acme", "13")

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: acme\nframework_version: \"6\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok {
		t.Fatal("GetFrameworkForDir resolved nothing")
	}
	if fw.Version != "10" || fw.DetectedVersion != "6" {
		t.Errorf("Version/DetectedVersion = %q/%q, want 10/6", fw.Version, fw.DetectedVersion)
	}
	if !fw.VersionGuessed {
		t.Error("VersionGuessed = false, want true for a legacy clamp")
	}
}

// An exact match runs what it says, so there is no second version to report.
func TestGetFrameworkForDir_exactVersionReportsNoOther(t *testing.T) {
	setConfigDir(t)
	installVersionedDef(t, "acme", "6")

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: acme\nframework_version: \"6\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok {
		t.Fatal("GetFrameworkForDir resolved nothing")
	}
	if fw.DetectedVersion != "" {
		t.Errorf("DetectedVersion = %q, want empty when the definition is the project's own version", fw.DetectedVersion)
	}
	if fw.VersionGuessed {
		t.Error("VersionGuessed = true, want false for an exact match")
	}
}
