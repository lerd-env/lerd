package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withFakeFetchHook installs a preset-fetch hook that records calls and, when
// body is non-empty, writes it to the store cache. It restores the previous hook
// on cleanup so tests don't leak a hook into one another.
func withFakeFetchHook(t *testing.T, body func(name string) string) *int {
	t.Helper()
	prev := presetFetchHook
	calls := 0
	presetFetchHook = func(name string) error {
		calls++
		if b := body(name); b != "" {
			return SaveStorePreset(name, []byte(b))
		}
		return nil
	}
	t.Cleanup(func() { presetFetchHook = prev })
	return &calls
}

// The seam must expose exactly the embedded bundle in Phase 0: every name it
// reports resolves, and every bundled preset is reported. This is the
// regression guard that routing ListPresets/LoadPreset/PresetExists and the
// default-preset + family indexes through the seam changed nothing.
func TestPresetNames_MatchEmbeddedAndResolve(t *testing.T) {
	names := presetNames()
	if len(names) == 0 {
		t.Fatal("presetNames() returned nothing")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("presetNames() not sorted/deduped: %q then %q", names[i-1], names[i])
		}
	}
	for _, name := range names {
		if !presetSourceExists(name) {
			t.Errorf("presetNames() reported %q but presetSourceExists is false", name)
		}
		if _, err := LoadPreset(name); err != nil {
			t.Errorf("LoadPreset(%q) = %v", name, err)
		}
	}
}

func TestPresetNames_CoversListPresets(t *testing.T) {
	metas, err := ListPresets()
	if err != nil {
		t.Fatalf("ListPresets() error = %v", err)
	}
	names := map[string]bool{}
	for _, n := range presetNames() {
		names[n] = true
	}
	for _, m := range metas {
		if !names[m.Name] {
			t.Errorf("ListPresets() surfaced %q but presetNames() omits it", m.Name)
		}
	}
}

func TestReadPresetBytes_UnknownMissing(t *testing.T) {
	if _, ok := readPresetBytes("definitely-not-a-preset"); ok {
		t.Error("readPresetBytes returned ok for unknown preset")
	}
	if presetSourceExists("definitely-not-a-preset") {
		t.Error("presetSourceExists true for unknown preset")
	}
}

// writeStorePreset drops a preset YAML into the store-cache dir for a tmp
// XDG_DATA_HOME the caller has already set via t.Setenv.
func writeStorePreset(t *testing.T, name, body string) {
	t.Helper()
	dir := StorePresetsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir store presets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write store preset: %v", err)
	}
}

// A valid preset in the store-cache dir supersedes the built-in of the same
// name (tested through the uncached readPresetBytes so the process-wide
// presetCache isn't polluted for other tests).
func TestReadPresetBytes_StoreCacheOverridesEmbed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "redis", "name: redis\nimage: example/store-redis:1\n")
	data, ok := readPresetBytes("redis")
	if !ok {
		t.Fatal("redis not found")
	}
	if !strings.Contains(string(data), "example/store-redis:1") {
		t.Errorf("store cache did not override embed, got: %s", data)
	}
}

// A store-cache preset that fails validation must never shadow a working
// built-in; readPresetBytes falls back to the embedded copy.
func TestReadPresetBytes_InvalidStoreFallsBackToEmbed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "redis", "name: redis\n") // no image, no versions => invalid
	data, ok := readPresetBytes("redis")
	if !ok {
		t.Fatal("expected embedded redis as fallback")
	}
	if !strings.Contains(string(data), "redis:7-alpine") {
		t.Errorf("expected embedded redis, got: %s", data)
	}
}

// A store-only preset (no built-in of that name) is enumerated and resolvable.
func TestPresetNames_UnionsStoreCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "phase2-store-only", "name: phase2-store-only\nimage: example/x:1\ndescription: store only\n")
	found := false
	for _, n := range presetNames() {
		if n == "phase2-store-only" {
			found = true
		}
	}
	if !found {
		t.Fatal("store-only preset missing from presetNames()")
	}
	p, err := LoadPreset("phase2-store-only")
	if err != nil {
		t.Fatalf("LoadPreset(store-only) = %v", err)
	}
	if p.Image != "example/x:1" {
		t.Errorf("store-only preset Image = %q, want example/x:1", p.Image)
	}
}

// EnsurePreset fetches an absent preset through the hook, after which the seam
// serves it locally.
func TestEnsurePreset_FetchesAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := withFakeFetchHook(t, func(name string) string {
		return "name: " + name + "\nimage: example/" + name + ":1\ndescription: fetched\n"
	})
	p, err := EnsurePreset("phase4-absent")
	if err != nil {
		t.Fatalf("EnsurePreset: %v", err)
	}
	if p.Image != "example/phase4-absent:1" {
		t.Errorf("fetched preset Image = %q", p.Image)
	}
	if *calls != 1 {
		t.Errorf("hook called %d times, want 1", *calls)
	}
}

// A built-in served from the embed bundle must resolve without any network fetch.
func TestEnsurePreset_EmbeddedNeverFetches(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := withFakeFetchHook(t, func(string) string { return "" })
	if _, err := EnsurePreset("redis"); err != nil {
		t.Fatalf("EnsurePreset(redis): %v", err)
	}
	if *calls != 0 {
		t.Errorf("hook called %d times for a built-in, want 0", *calls)
	}
}

// With no hook registered, an unknown preset is a plain error, not a panic.
func TestEnsurePreset_AbsentNoHook(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := presetFetchHook
	presetFetchHook = nil
	t.Cleanup(func() { presetFetchHook = prev })
	if _, err := EnsurePreset("phase4-nohook"); err == nil {
		t.Error("EnsurePreset must error for an unknown preset with no fetch hook")
	}
}

// A cached store preset older than the staleness window triggers a best-effort
// refresh; a fresh one does not.
func TestEnsurePreset_RefreshesStaleCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "phase4-stale", "name: phase4-stale\nimage: example/phase4-stale:1\n")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(StorePresetsDir(), "phase4-stale.yaml"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	calls := withFakeFetchHook(t, func(string) string { return "" })
	if _, err := EnsurePreset("phase4-stale"); err != nil {
		t.Fatalf("EnsurePreset: %v", err)
	}
	if *calls != 1 {
		t.Errorf("stale cache: hook called %d times, want 1 (refresh)", *calls)
	}

	// A freshly written cache preset must not trigger a refresh.
	writeStorePreset(t, "phase4-fresh", "name: phase4-fresh\nimage: example/phase4-fresh:1\n")
	calls2 := withFakeFetchHook(t, func(string) string { return "" })
	if _, err := EnsurePreset("phase4-fresh"); err != nil {
		t.Fatalf("EnsurePreset(fresh): %v", err)
	}
	if *calls2 != 0 {
		t.Errorf("fresh cache: hook called %d times, want 0", *calls2)
	}
}

func TestValidPresetName_RejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", "..", ".", "a/b", "../etc", `a\b`} {
		if validPresetName(bad) {
			t.Errorf("validPresetName(%q) = true, want false", bad)
		}
	}
	if !validPresetName("mysql") {
		t.Error("validPresetName(mysql) = false, want true")
	}
}
