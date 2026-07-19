package ui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestPhpIniScope checks the php.ini editor scope token resolves a bare version
// to the per-version file, "site:<name>" to a FrankenPHP site's own per-site
// file, and "shared" to the version-agnostic file, so the same editor serves all
// three.
func TestPhpIniScope(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	verFile := phpIniScopeFile("8.5").Path
	if verFile != config.PHPUserIniFile("8.5") {
		t.Errorf("version scope path = %q, want the per-version file", verFile)
	}
	siteFile := phpIniScopeFile("site:personal").Path
	if siteFile != config.SitePHPUserIniFile("personal") {
		t.Errorf("site scope path = %q, want the per-site file", siteFile)
	}
	sharedFile := phpIniScopeFile("shared").Path
	if sharedFile != config.SharedIniFile() {
		t.Errorf("shared scope path = %q, want the shared file", sharedFile)
	}
	if verFile == siteFile || verFile == sharedFile || siteFile == sharedFile {
		t.Error("version, site, and shared scopes must resolve to different files")
	}
	if !strings.Contains(siteFile, "/sites/personal/") {
		t.Errorf("site scope path %q should be under the per-site dir", siteFile)
	}
}
