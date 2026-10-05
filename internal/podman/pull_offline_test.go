package podman

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/imagepull"
)

// offlineExec swaps execCommand for a stub that records every podman argv and
// answers `image exists` according to imagePresent.
func offlineExec(t *testing.T, calls *[][]string, imagePresent bool) {
	t.Helper()
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		*calls = append(*calls, args)
		exit := 0
		if len(args) >= 2 && args[0] == "image" && args[1] == "exists" && !imagePresent {
			exit = 1
		}
		return fakeExec("", "", exit)(name, args...)
	}
}

func pulled(calls [][]string) bool {
	for _, args := range calls {
		if len(args) > 0 && args[0] == "pull" {
			return true
		}
	}
	return false
}

func TestRunPullOfflineKeepsAnImageThatIsAlreadyLocal(t *testing.T) {
	var calls [][]string
	offlineExec(t, &calls, true)
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })

	var out bytes.Buffer
	if err := runPull("redis:7-alpine", &out, &out); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if pulled(calls) {
		t.Error("offline pulled an image that was already in the local store")
	}
	if !strings.Contains(out.String(), "Offline") {
		t.Errorf("offline skip was not disclosed: %q", out.String())
	}
}

func TestRunPullOfflineStillPullsAMissingImage(t *testing.T) {
	var calls [][]string
	offlineExec(t, &calls, false)
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })

	var out bytes.Buffer
	if err := runPull("redis:7-alpine", &out, &out); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if !pulled(calls) {
		t.Error("offline refused to pull an image that is not present at all")
	}
}

func TestBuildFPMImageOfflineDefersARefreshOfAWorkingImage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var calls [][]string
	offlineExec(t, &calls, true)
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })

	var out bytes.Buffer
	built, err := buildFPMImage("8.4", false, false, nil, nil, nil, &out)
	if err != nil {
		t.Fatalf("buildFPMImage: %v", err)
	}
	if built {
		t.Error("offline rebuilt an image that already exists")
	}
	for _, args := range calls {
		if len(args) > 0 && args[0] == "build" {
			t.Error("offline ran a build that would re-download the whole base")
		}
	}
	if !strings.Contains(out.String(), "php:rebuild") {
		t.Errorf("deferral did not point at the escape hatch: %q", out.String())
	}
}

// A stale image that exists is kept offline, so it must not be disclosed as a
// rebuild; online the same image is rebuilt and has to be.
func TestFPMImageWouldBuildFollowsTheOfflineDeferral(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var calls [][]string
	offlineExec(t, &calls, true)

	if !FPMImageWouldBuild("8.4") {
		t.Error("online, a stale image was not reported as a rebuild")
	}
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })
	if FPMImageWouldBuild("8.4") {
		t.Error("offline, a stale image that is kept was reported as a rebuild")
	}
}

func TestFPMImageWouldBuildOfflineStillBuildsAMissingImage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var calls [][]string
	offlineExec(t, &calls, false)
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })

	if !FPMImageWouldBuild("8.4") {
		t.Error("offline, a missing image was not reported as a build")
	}
}
