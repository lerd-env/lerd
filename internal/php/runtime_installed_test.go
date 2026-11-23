package php

import (
	"strings"
	"testing"
)

// The dashboard listed the versions one way and asked about them another, so a
// version installed only as a host build was offered and then 404'd, while one
// that exists only as a container unit was reported as stopped. Both lists are
// real; which one applies is decided by the runtime.
func TestInstalledForRuntime(t *testing.T) {
	prevNative, prevHost, prevContainer := nativeRuntimeFn, nativeInstalledFn, containerInstalledFn
	nativeInstalledFn = func() []string { return []string{"8.1", "8.2"} }
	containerInstalledFn = func() ([]string, error) { return []string{"8.2", "8.6"}, nil }
	t.Cleanup(func() {
		nativeRuntimeFn, nativeInstalledFn, containerInstalledFn = prevNative, prevHost, prevContainer
	})

	nativeRuntimeFn = func() bool { return true }
	got, err := InstalledForRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "8.1,8.2" {
		t.Errorf("native = %v, want the host builds", got)
	}

	nativeRuntimeFn = func() bool { return false }
	got, err = InstalledForRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "8.2,8.6" {
		t.Errorf("container = %v, want the FPM units", got)
	}
}

// stubRuntimeInstalled pins the runtime and what each runtime has installed.
func stubRuntimeInstalled(t *testing.T, native bool, host, container []string) {
	t.Helper()
	prevNative, prevHost, prevContainer := nativeRuntimeFn, nativeInstalledFn, containerInstalledFn
	nativeRuntimeFn = func() bool { return native }
	nativeInstalledFn = func() []string { return host }
	containerInstalledFn = func() ([]string, error) { return container, nil }
	t.Cleanup(func() {
		nativeRuntimeFn, nativeInstalledFn, containerInstalledFn = prevNative, prevHost, prevContainer
	})
}

// Going native keeps the container units, which still list 8.6, but nothing on
// the host can run it. A project asking for ^8.3 must resolve to the newest
// host build, not to a version that fails as soon as composer runs.
func TestComposerConstraintResolvesForTheActiveRuntime(t *testing.T) {
	host := []string{"8.5"}
	container := []string{"8.2", "8.3", "8.4", "8.5", "8.6"}
	cases := []struct {
		name   string
		native bool
		want   string
	}{
		{"native", true, "8.5"},
		{"container", false, "8.6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			stubRuntimeInstalled(t, c.native, host, container)
			dir := t.TempDir()
			writeFile(t, dir, "composer.json", `{"require":{"php":"^8.3"}}`)

			got, err := DetectVersion(dir)
			if err != nil {
				t.Fatalf("DetectVersion: %v", err)
			}
			if got != c.want {
				t.Errorf("DetectVersion = %q, want %q", got, c.want)
			}
			if best := BestInstalledFor(">=8.3"); best != c.want {
				t.Errorf("BestInstalledFor = %q, want %q", best, c.want)
			}
		})
	}
}
