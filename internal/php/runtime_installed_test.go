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
