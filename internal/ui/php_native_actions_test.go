package ui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
