package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/tools"
)

// Under the native runtime there is no image to inspect, and the fix doctor
// points at for a stale one refuses outright. What can be wrong instead is a
// build that has fallen behind the published patch.
func TestNativeBuildFinding(t *testing.T) {
	cases := []struct {
		name               string
		installed, pinned  string
		wantStatus, wantIn string
	}{
		{"up to date", "8.4.25", "8.4.25", "ok", ""},
		{"a newer patch is published", "8.4.24", "8.4.25", "warn", "8.4.25"},
		// With no pin reachable there is nothing to compare against, and an
		// installed build is not suspect just because the network is down.
		{"no pin available", "8.4.25", "", "ok", ""},
		// A binary installed before lerd recorded patches has no stamp. It is
		// on disk and serving, so there is nothing to compare it against.
		{"present but unstamped", "", "8.4.25", "ok", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, detail := nativeBuildFinding(c.installed, c.pinned)
			if status != c.wantStatus {
				t.Errorf("status = %q, want %q (detail %q)", status, c.wantStatus, detail)
			}
			if c.wantIn != "" && !strings.Contains(detail, c.wantIn) {
				t.Errorf("detail %q should mention %q", detail, c.wantIn)
			}
		})
	}
}

// recorder collects what the doctor reporters were called with.
type recorder struct{ oks, warns []string }

func (r *recorder) ok(label string)        { r.oks = append(r.oks, label) }
func (r *recorder) warn(label, msg string) { r.warns = append(r.warns, label+": "+msg) }

// Doctor judged the container-registered versions against host builds, so a
// version that exists only as a quadlet failed as "not installed" and a
// prerelease with nothing published failed as unbuildable, neither of which
// this runtime is being asked to serve.
func TestNativePHPFindingsCoverOnlyTheInstalledBuilds(t *testing.T) {
	var r recorder
	pins := &tools.Manifest{Tools: map[string]tools.Tool{
		nativeTool("8.4"): {Version: "8.4.25"},
	}}

	printNativePHPFindings([]string{"8.4"}, pins,
		func(v string) string { return "8.4.25" }, r.ok, r.warn)

	if len(r.oks) != 1 || !strings.Contains(r.oks[0], "8.4") {
		t.Errorf("oks = %v, want the installed build reported once", r.oks)
	}
	if len(r.warns) != 0 {
		t.Errorf("warns = %v, want nothing for an up-to-date build", r.warns)
	}
}

func TestNativePHPFindingsWarnOnANewerBuild(t *testing.T) {
	var r recorder
	pins := &tools.Manifest{Tools: map[string]tools.Tool{
		nativeTool("8.4"): {Version: "8.4.25"},
	}}

	printNativePHPFindings([]string{"8.4"}, pins,
		func(v string) string { return "8.4.24" }, r.ok, r.warn)

	if len(r.warns) != 1 || !strings.Contains(r.warns[0], "php:update 8.4") {
		t.Errorf("warns = %v, want the update command named", r.warns)
	}
}

// Version Info reports what is installed, which is true of a host build too.
// It used to read the same slice the image loop emptied on the native runtime,
// so a machine with five builds serving reported "PHP installed (none)".
func TestPHPVersionsForDoctorKeepsNativeBuildsReportable(t *testing.T) {
	reported, images := phpVersionsForDoctor(true, []string{"8.2", "8.6"}, []string{"8.3", "8.4"})
	if len(reported) != 2 || reported[0] != "8.3" {
		t.Errorf("reported = %v, want the host builds", reported)
	}
	if len(images) != 0 {
		t.Errorf("images = %v, want nothing to inspect on the native runtime", images)
	}
}

func TestPHPVersionsForDoctorOnTheContainerRuntime(t *testing.T) {
	reported, images := phpVersionsForDoctor(false, []string{"8.2", "8.6"}, []string{"8.3"})
	if len(reported) != 2 || len(images) != 2 {
		t.Errorf("reported = %v, images = %v, want the registered versions for both", reported, images)
	}
}
