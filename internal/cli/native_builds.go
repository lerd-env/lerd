package cli

import (
	"context"
	"io"
	"os"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// nativeBuildState is what is known about one version's host build.
type nativeBuildState struct {
	present   bool   // the binary is on disk
	installed string // the patch recorded for it, empty when it predates stamping
	pinned    string // the patch lerd publishes, empty when none is reachable
}

// nativeBuildsToFetch picks the versions whose host build has to be downloaded:
// the ones with no binary, and the ones a newer patch has been published for.
//
// A build with no stamp is left alone. It is on disk and serving, and lerd only
// began recording patches later, so treating it as unknown would re-download
// every version on the first install after an upgrade. A version with no pin is
// left alone too: nothing is published to fetch.
func nativeBuildsToFetch(versions []string, state func(string) nativeBuildState) []string {
	var out []string
	for _, v := range versions {
		s := state(v)
		if s.pinned == "" {
			continue
		}
		if !s.present {
			out = append(out, v)
			continue
		}
		if s.installed != "" && s.installed != s.pinned {
			out = append(out, v)
		}
	}
	return out
}

// nativeBuildStateFor reads the state of one version from disk and the pins.
func nativeBuildStateFor(m *tools.Manifest, version string) nativeBuildState {
	_, err := os.Stat(nativephp.BinaryPath(version))
	return nativeBuildState{
		present:   err == nil,
		installed: tools.InstalledVersion(nativeTool(version)),
		pinned:    m.Tools[nativeTool(version)].Version,
	}
}

// ensureNativePHPBuilds fetches the host builds an install needs, which is what
// "build the PHP images" means once PHP runs on the host. Nothing to fetch
// prints nothing, so an install that changes none of them stays quiet.
func ensureNativePHPBuilds(versions []string) {
	var supported []string
	for _, v := range versions {
		if nativephp.Supported(v) {
			supported = append(supported, v)
		}
	}
	if len(supported) == 0 {
		return
	}
	pins := &pinnedTools{m: tools.Load(context.Background())}
	want := nativeBuildsToFetch(supported, func(v string) nativeBuildState {
		return nativeBuildStateFor(pins.m, v)
	})
	if len(want) == 0 {
		return
	}

	feedback.Header("Fetching PHP builds")
	jobs := make([]BuildJob, len(want))
	for i, v := range want {
		ver := v
		jobs[i] = BuildJob{
			Label: "PHP " + ver,
			Run: func(w io.Writer) error {
				_, err := installNativePHP(pins, ver, w)
				return err
			},
		}
	}
	RunParallel(jobs) //nolint:errcheck
}
