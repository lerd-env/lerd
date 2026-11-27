package ui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// stubNativeRuntime forces the runtime the PHP actions branch on, so a test can
// drive the host path without a global config on disk.
func stubNativeRuntime(t *testing.T, native bool) {
	t.Helper()
	prev := nativeRuntimeActive
	nativeRuntimeActive = func() bool { return native }
	t.Cleanup(func() { nativeRuntimeActive = prev })
}

func stubNativeRemove(t *testing.T, err error) *string {
	t.Helper()
	var seen string
	prev := removeNativePHP
	removeNativePHP = func(v string) error {
		seen = v
		return err
	}
	t.Cleanup(func() { removeNativePHP = prev })
	return &seen
}

func stubNativeUpdate(t *testing.T, err error) *string {
	t.Helper()
	var seen string
	prev := updateNativePHP
	updateNativePHP = func(v string, w io.Writer) error {
		seen = v
		io.WriteString(w, "fetching php "+v+"\n")
		return err
	}
	t.Cleanup(func() { updateNativePHP = prev })
	return &seen
}

func postPHPAction(version, action string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	path := "/api/php-versions/" + version + "/" + action
	handlePHPVersionAction(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

// Removing a version under the native runtime has to take the host build.
// Tearing down the quadlet instead reported success and left the version in the
// list, because that list is read from the binaries on disk.
func TestRemoveUnderNativeRemovesTheHostBuild(t *testing.T) {
	stubNativeRuntime(t, true)
	removed := stubNativeRemove(t, nil)

	rec := postPHPAction("8.1", "remove")
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want ok:true", rec.Body.String())
	}
	if *removed != "8.1" {
		t.Errorf("removed %q, want the native build for 8.1", *removed)
	}
}

func TestRemoveUnderNativeReportsFailure(t *testing.T) {
	stubNativeRuntime(t, true)
	stubNativeRemove(t, errors.New("permission denied"))

	rec := postPHPAction("8.1", "remove")
	if !strings.Contains(rec.Body.String(), "permission denied") {
		t.Errorf("body = %s, want the failure surfaced", rec.Body.String())
	}
}

// The container runtime keeps the quadlet teardown, so the native helper must
// stay untouched there.
func TestRemoveUnderContainerLeavesTheHostBuildAlone(t *testing.T) {
	stubNativeRuntime(t, false)
	removed := stubNativeRemove(t, nil)
	prev := teardownPHPFPMFn
	var torndown string
	teardownPHPFPMFn = func(v string) error {
		torndown = v
		return nil
	}
	t.Cleanup(func() { teardownPHPFPMFn = prev })

	postPHPAction("8.1", "remove")
	if torndown != "8.1" {
		t.Errorf("tore down %q, want the 8.1 container", torndown)
	}
	if *removed != "" {
		t.Errorf("removed the native build %q on the container runtime", *removed)
	}
}

// Under the native runtime there is no image to rebuild: an update downloads
// the published build and restarts the pool.
func TestRebuildUnderNativeUpdatesTheHostBuild(t *testing.T) {
	stubNativeRuntime(t, true)
	updated := stubNativeUpdate(t, nil)

	rec := postPHPAction("8.1", "rebuild")
	if *updated != "8.1" {
		t.Errorf("updated %q, want the native build for 8.1", *updated)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "fetching php 8.1") {
		t.Errorf("body = %s, want the update log streamed", body)
	}
	if !strings.Contains(body, `"ok":true`) {
		t.Errorf("body = %s, want a done payload", body)
	}
}

func TestRebuildUnderNativeStreamsTheFailure(t *testing.T) {
	stubNativeRuntime(t, true)
	stubNativeUpdate(t, errors.New("no build published"))

	rec := postPHPAction("8.1", "rebuild")
	body := rec.Body.String()
	if !strings.Contains(body, "no build published") || !strings.Contains(body, `"ok":false`) {
		t.Errorf("body = %s, want the failure in the done payload", body)
	}
}

func postPHPInstall(version string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handlePHPInstall(rec, httptest.NewRequest(http.MethodPost, "/api/php-versions/install?version="+version, nil))
	return rec
}

func stubPHPInstall(t *testing.T, err error) *string {
	t.Helper()
	var seen string
	prev := installPHPVersionFn
	installPHPVersionFn = func(v string, _ io.Writer) error {
		seen = v
		return err
	}
	t.Cleanup(func() { installPHPVersionFn = prev })
	return &seen
}

// Installing from the dashboard under the native runtime fetches the published
// build. It used to refuse and tell the user to leave native mode, which left a
// fresh native install with no way to get PHP from the UI.
func TestInstallUnderNativeFetchesTheHostBuild(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubNativeRuntime(t, true)
	installed := stubPHPInstall(t, nil)

	rec := postPHPInstall("8.4")
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want ok:true", rec.Body.String())
	}
	if *installed != "8.4" {
		t.Errorf("installed %q, want 8.4", *installed)
	}
}

// A failed native fetch has no quadlet to roll back, so the container teardown
// must not run against a version the container runtime never had.
func TestFailedNativeInstallLeavesContainersAlone(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubNativeRuntime(t, true)
	stubPHPInstall(t, errors.New("download failed"))
	prev := teardownPHPFPMFn
	torndown := ""
	teardownPHPFPMFn = func(v string) error { torndown = v; return nil }
	t.Cleanup(func() { teardownPHPFPMFn = prev })

	rec := postPHPInstall("8.4")
	if !strings.Contains(rec.Body.String(), "download failed") {
		t.Errorf("body = %s, want the failure surfaced", rec.Body.String())
	}
	if torndown != "" {
		t.Errorf("tore down the %s container on the native runtime", torndown)
	}
}

// Under the native runtime "installed" means the host build is on disk, so a
// version whose binaries are there is not offered again.
func TestFullyInstalledFollowsTheNativeBuilds(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubNativeRuntime(t, true)
	bin := config.BinDir()
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"php-native-8.4", "php-native-fpm-8.4"} {
		if err := os.WriteFile(filepath.Join(bin, name), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := fullyInstalledPHPVersions(); !slices.Equal(got, []string{"8.4"}) {
		t.Errorf("fully installed = %v, want [8.4]", got)
	}
}

// stubNativePins publishes a host build of each version for this platform only.
func stubNativePins(t *testing.T, versions ...string) {
	t.Helper()
	m := &tools.Manifest{Tools: map[string]tools.Tool{}}
	for _, v := range versions {
		m.Tools[nativephp.ToolName(v)] = tools.Tool{
			Version: v + ".0",
			URL:     "https://example.test/{asset}",
			Assets:  map[string]string{runtime.GOOS + "/" + runtime.GOARCH: "php.tar.gz"},
		}
	}
	prev := nativePins
	nativePins = func() *tools.Manifest { return m }
	t.Cleanup(func() { nativePins = prev })
}

// Versions below the native minimum have no build, so they are not offered.
func TestInstallableUnderNativeSkipsUnsupportedVersions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubNativeRuntime(t, true)
	stubNativePins(t, "7.4", "8.4")
	rec := httptest.NewRecorder()
	handlePHPInstallable(rec, httptest.NewRequest(http.MethodGet, "/api/php-installable", nil))
	if body := rec.Body.String(); strings.Contains(body, `"7.4"`) || !strings.Contains(body, `"8.4"`) {
		t.Errorf("installable = %s, want 8.4 offered and 7.4 left out", body)
	}
}

// A supported version lerd publishes no host build of for this platform can only
// fail to install, so it is not offered either.
func TestInstallableUnderNativeSkipsVersionsWithNoBuild(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubNativeRuntime(t, true)
	stubNativePins(t, "8.4", "8.5")
	m := nativePins()
	other := "linux/amd64"
	if runtime.GOOS+"/"+runtime.GOARCH == other {
		other = "darwin/arm64"
	}
	m.Tools[nativephp.ToolName("8.6")] = tools.Tool{Version: "8.6.0", URL: "https://example.test/{asset}", Assets: map[string]string{other: "php.tar.gz"}}

	rec := httptest.NewRecorder()
	handlePHPInstallable(rec, httptest.NewRequest(http.MethodGet, "/api/php-installable", nil))
	var got []string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"8.4", "8.5"}) {
		t.Errorf("installable = %v, want [8.4 8.5]", got)
	}
}
