package ui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The site's PHP log tab streams a unit name. Under the native runtime there is
// no container to read from, and the host FPM writes to its own launchd log, so
// the tab has to be pointed there instead of at a container that is stopped.
func TestPHPLogUnit(t *testing.T) {
	if got := phpLogUnit(config.Site{Name: "shop"}, "8.4", false); got != "lerd-php84-fpm" {
		t.Errorf("container mode = %q, want lerd-php84-fpm", got)
	}
	if got := phpLogUnit(config.Site{Name: "shop"}, "8.4", true); got != "lerd-native-php84" {
		t.Errorf("native mode = %q, want lerd-native-php84", got)
	}
}

// A site the native runtime never touches must keep reading its own container,
// whatever the install-wide mode says.
func TestPHPLogUnitLeavesOwnContainerSitesAlone(t *testing.T) {
	for _, site := range []config.Site{
		{Name: "shop", Runtime: "frankenphp"},
		{Name: "shop", Runtime: "fpm-custom"},
		{Name: "shop", ContainerPort: 8080},
		{Name: "shop", HostPort: 3000},
	} {
		if got := phpLogUnit(site, "8.4", true); got == "lerd-native-php84" {
			t.Errorf("%+v must not read the native log, got %q", site, got)
		}
	}
}

// The dashboard's PHP health dots read this. Under the native runtime the
// containers are stopped on purpose, so reading container state would paint
// every version grey while PHP is in fact serving.
func TestPHPRunningFollowsTheRuntime(t *testing.T) {
	var askedContainer, askedNative []string
	container := func(v string) bool { askedContainer = append(askedContainer, v); return true }
	native := func(v string) bool { askedNative = append(askedNative, v); return true }

	if !phpVersionRunning("8.4", false, container, native) {
		t.Error("container mode should report the container")
	}
	if len(askedContainer) != 1 || len(askedNative) != 0 {
		t.Errorf("container mode must ask the container only, got c=%v n=%v", askedContainer, askedNative)
	}

	askedContainer, askedNative = nil, nil
	if !phpVersionRunning("8.4", true, container, native) {
		t.Error("native mode should report the host listener")
	}
	if len(askedNative) != 1 || len(askedContainer) != 0 {
		t.Errorf("native mode must ask the listener only, got c=%v n=%v", askedContainer, askedNative)
	}
}

// The settings page must offer what can actually serve. Under the native
// runtime that is the native binaries, not the container images, otherwise it
// lists versions with nothing behind them.
func TestInstalledPHPVersionsFollowsTheRuntime(t *testing.T) {
	containerList := func() []string { return []string{"8.1", "8.2"} }
	nativeList := func() []string { return []string{"8.4", "8.5"} }

	if got := installedPHPVersions(false, containerList, nativeList); got[0] != "8.1" {
		t.Errorf("container mode = %v, want the container versions", got)
	}
	if got := installedPHPVersions(true, containerList, nativeList); got[0] != "8.4" {
		t.Errorf("native mode = %v, want the native versions", got)
	}
	// Never nil: the UI renders the list directly.
	if got := installedPHPVersions(true, containerList, func() []string { return nil }); got == nil {
		t.Error("expected an empty slice rather than nil")
	}
}

// Installing from the settings page builds a container image. Under the native
// runtime that is the wrong artifact entirely, and building one silently would
// leave the user waiting on an image nothing will serve from.
func TestPHPInstallRefusedUnderNative(t *testing.T) {
	if err := nativeInstallRefusal(true, "8.6"); err == nil {
		t.Error("native runtime must refuse a container image build")
	} else if !strings.Contains(err.Error(), "8.6") {
		t.Errorf("the refusal should name the version, got: %v", err)
	}
	if err := nativeInstallRefusal(false, "8.6"); err != nil {
		t.Errorf("container mode must allow the install, got: %v", err)
	}
}

// The dashboard hides the runtime toggle where it cannot apply. Gating on the
// OS alone would still offer it on an Intel Mac, where no binary exists, and
// the switch would fail after the user had already chosen it.
func TestNativeRuntimeApplies(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         bool
	}{
		{"darwin", "arm64", true},
		{"darwin", "amd64", false},
		{"linux", "arm64", false},
		{"linux", "amd64", false},
	}
	for _, c := range cases {
		if got := nativeRuntimeApplies(c.goos, c.goarch); got != c.want {
			t.Errorf("%s/%s = %v, want %v", c.goos, c.goarch, got, c.want)
		}
	}
}
