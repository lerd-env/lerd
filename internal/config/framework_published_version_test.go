package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStoreIndex caches a store index publishing one framework at the given
// versions, the state a machine reaches as soon as it has talked to the store
// once, whether or not it has installed any definition.
func writeStoreIndex(t *testing.T, name, latest string, versions ...string) {
	t.Helper()
	if err := os.MkdirAll(StoreFrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	quoted := ""
	for i, v := range versions {
		if i > 0 {
			quoted += ","
		}
		quoted += `"` + v + `"`
	}
	index := `{"frameworks":[{"name":"` + name + `","label":"` + name + `","versions":[` + quoted +
		`],"latest":"` + latest + `","detect":[{"file":"acme.lock"}]}]}`
	if err := os.WriteFile(StoreIndexFile(), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLerdYAML(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// recordFetches swaps the store fetch hook for one that records what was asked
// for and installs a definition only for the versions the store publishes.
func recordFetches(t *testing.T, published ...string) *[]string {
	t.Helper()
	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })

	asked := []string{}
	has := map[string]bool{}
	for _, v := range published {
		has[v] = true
	}
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		asked = append(asked, version)
		if !has[version] {
			return nil, os.ErrNotExist
		}
		fw := &Framework{Name: name, Label: "Acme", Version: version, PublicDir: "public"}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}
	return &asked
}

// A project newer than every published definition resolved no framework at all
// on a machine that had never installed one: the fetch asked for the project's
// own version, the store has no such file, and nothing then asked for the
// newest version it does have.
func TestGetFrameworkForDir_AboveTheNewestPublishedVersion(t *testing.T) {
	setConfigDir(t)
	writeStoreIndex(t, "acme", "6", "6", "5")
	asked := recordFetches(t, "6", "5")

	dir := t.TempDir()
	writeLerdYAML(t, dir, "framework: acme\nframework_version: \"7\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok {
		t.Fatal("GetFrameworkForDir resolved nothing for a project above the newest published version")
	}
	if fw.Version != "6" {
		t.Errorf("Version = %q, want 6 (the newest published)", fw.Version)
	}
	// The project's PHP range is the borrowed definition's, the same as it is
	// for a future version served off disk.
	if fw.VersionGuessed {
		t.Error("VersionGuessed = true, want false for a future-version fallback")
	}
	for _, v := range *asked {
		if v == "7" {
			t.Errorf("asked the store for version 7, which its index does not publish (asked %v)", *asked)
		}
	}
}

// The legacy clamp has to work off the published list too, or a machine that
// has installed nothing serves a legacy project the newest definition instead
// of the oldest.
func TestGetFrameworkForDir_BelowTheOldestPublishedVersion(t *testing.T) {
	setConfigDir(t)
	writeStoreIndex(t, "acme", "6", "6", "5")
	recordFetches(t, "6", "5")

	dir := t.TempDir()
	writeLerdYAML(t, dir, "framework: acme\nframework_version: \"3\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok {
		t.Fatal("GetFrameworkForDir resolved nothing for a legacy project")
	}
	if fw.Version != "5" {
		t.Errorf("Version = %q, want 5 (the oldest published)", fw.Version)
	}
	if !fw.VersionGuessed || fw.DetectedVersion != "3" {
		t.Errorf("guessed=%v detected=%q, want a guessed definition reporting version 3", fw.VersionGuessed, fw.DetectedVersion)
	}
}

// An exact published version is still fetched as itself.
func TestGetFrameworkForDir_ExactPublishedVersionIsFetched(t *testing.T) {
	setConfigDir(t)
	writeStoreIndex(t, "acme", "6", "6", "5")
	asked := recordFetches(t, "6", "5")

	dir := t.TempDir()
	writeLerdYAML(t, dir, "framework: acme\nframework_version: \"5\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok || fw.Version != "5" {
		t.Fatalf("GetFrameworkForDir = %+v (ok=%v), want the 5 definition", fw, ok)
	}
	if len(*asked) == 0 || (*asked)[0] != "5" {
		t.Errorf("asked %v, want the exact version first", *asked)
	}
}

// With no cached index there is nothing to rule a version out, so the fetch
// still asks for it: an install that has never reached the store must not lose
// the one request that would get it a definition.
func TestGetFrameworkForDir_NoIndexStillAsksForTheDetectedVersion(t *testing.T) {
	setConfigDir(t)
	asked := recordFetches(t, "7")

	dir := t.TempDir()
	writeLerdYAML(t, dir, "framework: acme\nframework_version: \"7\"\n")

	fw, ok := GetFrameworkForDir("acme", dir)
	if !ok || fw.Version != "7" {
		t.Fatalf("GetFrameworkForDir = %+v (ok=%v), want the 7 definition", fw, ok)
	}
	if len(*asked) == 0 || (*asked)[0] != "7" {
		t.Errorf("asked %v, want version 7", *asked)
	}
}

// The published list is what a fresh machine has to reason about, so the clamp
// reads it alongside whatever is installed on disk.
func TestClampFrameworkVersion_ReadsThePublishedList(t *testing.T) {
	setConfigDir(t)
	writeStoreIndex(t, "acme", "6", "6", "5")

	if got := clampFrameworkVersion("acme", "3"); got != "5" {
		t.Errorf("clampFrameworkVersion(3) = %q, want 5 from the published list", got)
	}
	if got := clampFrameworkVersion("acme", "7"); got != "" {
		t.Errorf("clampFrameworkVersion(7) = %q, want empty above the newest", got)
	}
}

// An installed definition the index does not list still counts: a user-side
// install is not made invisible by a stale or partial index.
func TestClampFrameworkVersion_UnionsDiskAndPublished(t *testing.T) {
	setConfigDir(t)
	writeStoreIndex(t, "acme", "6", "6")
	if err := os.MkdirAll(StoreFrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "name: acme\nlabel: Acme\nversion: \"4\"\npublic_dir: public\n"
	if err := os.WriteFile(filepath.Join(StoreFrameworksDir(), "acme@4.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := clampFrameworkVersion("acme", "2"); got != "4" {
		t.Errorf("clampFrameworkVersion(2) = %q, want 4 (installed, below the published range)", got)
	}
}
