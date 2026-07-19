package podman

import (
	"os/exec"
	"testing"
)

func resetPHPVerCache() {
	phpVerMu.Lock()
	phpVerCache = map[string]phpVerEntry{}
	phpVerInflight = map[string]bool{}
	phpVerMu.Unlock()
}

// A version's full patch is probed from the image once and cached, keyed on the
// image's containerfile hash.
func TestRefreshPHPVersion_probesAndCaches(t *testing.T) {
	origLabel, origExec := imageLabelFn, execCommand
	t.Cleanup(func() { imageLabelFn = origLabel; execCommand = origExec; resetPHPVerCache() })
	resetPHPVerCache()

	imageLabelFn = func(_, _ string) string { return "hash1" }
	probes := 0
	execCommand = func(name string, arg ...string) *exec.Cmd {
		probes++
		return exec.Command("printf", "8.5.1")
	}

	refreshPHPVersion("8.5")
	phpVerMu.Lock()
	got := phpVerCache["8.5"].patch
	phpVerMu.Unlock()
	if got != "8.5.1" {
		t.Fatalf("cached patch = %q, want 8.5.1", got)
	}

	// A second call with the same image hash must not re-probe.
	refreshPHPVersion("8.5")
	if probes != 1 {
		t.Errorf("probed %d times, want 1 (cache should serve the unchanged image)", probes)
	}

	// A new image hash (a rebuild) must re-probe.
	imageLabelFn = func(_, _ string) string { return "hash2" }
	execCommand = func(name string, arg ...string) *exec.Cmd { return exec.Command("printf", "8.5.2") }
	refreshPHPVersion("8.5")
	phpVerMu.Lock()
	got = phpVerCache["8.5"].patch
	phpVerMu.Unlock()
	if got != "8.5.2" {
		t.Errorf("after rebuild cached patch = %q, want 8.5.2", got)
	}
}

// An unbuilt image (no hash label) yields no patch and no probe.
func TestRefreshPHPVersion_unbuiltImageIsNoop(t *testing.T) {
	origLabel, origExec := imageLabelFn, execCommand
	t.Cleanup(func() { imageLabelFn = origLabel; execCommand = origExec; resetPHPVerCache() })
	resetPHPVerCache()

	imageLabelFn = func(_, _ string) string { return "" }
	probed := false
	execCommand = func(name string, arg ...string) *exec.Cmd { probed = true; return exec.Command("true") }

	refreshPHPVersion("8.5")
	if probed {
		t.Error("must not probe an image that is not built")
	}
	phpVerMu.Lock()
	_, ok := phpVerCache["8.5"]
	phpVerMu.Unlock()
	if ok {
		t.Error("must not cache anything for an unbuilt image")
	}
}
