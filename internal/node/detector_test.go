package node

import (
	"os"
	"path/filepath"
	"testing"
)

// ── isNumericVersion ─────────────────────────────────────────────────────────

func TestIsNumericVersion(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"22", true},
		{"18", true},
		{"", false},
		{"system", false},
		{"lts/iron", false},
		{"v22", false},
		{"22.x", false},
		{"22.1.0", false},
	}
	for _, c := range cases {
		got := isNumericVersion(c.in)
		if got != c.want {
			t.Errorf("isNumericVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── extractMajor ─────────────────────────────────────────────────────────────

func TestExtractMajor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"22", "22"},
		{"18.12.0", "18"},
		{"20.1", "20"},
		{"system", "system"},
		{"", ""},
	}
	for _, c := range cases {
		got := extractMajor(c.in)
		if got != c.want {
			t.Errorf("extractMajor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── parseNodeConstraint ──────────────────────────────────────────────────────

func TestParseNodeConstraint(t *testing.T) {
	cases := []struct{ in, want string }{
		{">=18", "18"},
		{"^20.0.0", "20"},
		{"18.x", "18"},
		{">=16.0.0 <20", "16"},
		{"*", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := parseNodeConstraint(c.in)
		if got != c.want {
			t.Errorf("parseNodeConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── DetectVersion ────────────────────────────────────────────────────────────

func TestDetectVersion_nvmrc(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("v18.12.0\n"), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "18" {
		t.Errorf("got %q, want %q", got, "18")
	}
}

func TestDetectVersion_nvmrc_majorOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20\n"), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "20" {
		t.Errorf("got %q, want %q", got, "20")
	}
}

// Regression: .nvmrc containing "system" should fall through, not propagate "system".
func TestDetectVersion_nvmrc_system_fallsThrough(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("system\n"), 0644)
	// No .node-version, no package.json → should reach global default ("22")
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	// "system" is non-numeric and must not be returned
	if got == "system" {
		t.Error("DetectVersion must not return \"system\" from .nvmrc")
	}
}

func TestDetectVersion_nodeVersion_file(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".node-version"), []byte("v20.5.0\n"), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "20" {
		t.Errorf("got %q, want %q", got, "20")
	}
}

func TestDetectVersion_nodeVersion_precedence(t *testing.T) {
	// .nvmrc says "system" (invalid), .node-version says 16 → should use 16
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("system\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".node-version"), []byte("16\n"), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "16" {
		t.Errorf("got %q, want %q", got, "16")
	}
}

func TestDetectVersion_packageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":">=18.0.0"}}`), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "18" {
		t.Errorf("got %q, want %q", got, "18")
	}
}

func TestDetectVersion_noFiles_returnsDefault(t *testing.T) {
	dir := t.TempDir()
	// XDG_CONFIG_HOME points to a dir with no config.yaml → defaultConfig() → "22"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "22" {
		t.Errorf("got %q, want %q", got, "22")
	}
}

// ── UnpinnedVersion ──────────────────────────────────────────────────────────

// The wizard offers the version a blank Node answer accepts, and the field it
// offers it in writes the .lerd.yaml pin, so an existing pin must not be the
// answer to that question.
func TestUnpinnedVersion_ignoresLerdYAMLPin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("node_version: \"18\"\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20\n"), 0644)

	if pinned, err := DetectVersion(dir); err != nil || pinned != "18" {
		t.Fatalf("guard sanity: DetectVersion = %q, %v, want %q", pinned, err, "18")
	}
	got, source := UnpinnedVersion(dir)
	if got != "20" || source != ".nvmrc" {
		t.Errorf("UnpinnedVersion = %q from %q, want %q from %q", got, source, "20", ".nvmrc")
	}
}

func TestUnpinnedVersion_namesTheSource(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		want, source string
	}{
		{"nvmrc", map[string]string{".nvmrc": "20\n"}, "20", ".nvmrc"},
		{"node-version file", map[string]string{".node-version": "v18.5.0\n"}, "18", ".node-version"},
		{"package.json engines", map[string]string{"package.json": `{"engines":{"node":">=24"}}`}, "24", "package.json"},
		{"nothing declared", nil, "22", "the lerd default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			for name, body := range tc.files {
				os.WriteFile(filepath.Join(dir, name), []byte(body), 0644)
			}
			got, source := UnpinnedVersion(dir)
			if got != tc.want || source != tc.source {
				t.Errorf("UnpinnedVersion = %q from %q, want %q from %q", got, source, tc.want, tc.source)
			}
		})
	}
}

func TestDetectVersion_nvmrcOverridesPackageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20\n"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":">=18"}}`), 0644)
	got, err := DetectVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	// .nvmrc has priority over package.json
	if got != "20" {
		t.Errorf("got %q, want %q", got, "20")
	}
}

// A worktree pinning its own Node in the untracked override file gets it, the
// same way .lerd.yaml's pin outranks .nvmrc.
func TestPinnedVersion_LocalOverride(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		".lerd.yaml":       "node_version: \"20\"\n",
		".lerd.local.yaml": "node_version: \"22\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := pinnedVersion(dir); got != "22" {
		t.Errorf("pinnedVersion = %q, want 22 from .lerd.local.yaml", got)
	}
}
