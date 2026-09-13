package sitedoctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// checkHostMountedPath warns when a path the framework declares sits on the
// macOS bind mount, where every stat and write in it crosses virtiofs. The
// framework names the paths and carries its own remedy in the detail, since
// only it knows where its compiled cache goes and how to move it.
func checkHostMountedPath(path string, spec config.DoctorCheck) (Check, bool) {
	return hostMountedPathOn(runtime.GOOS, siteServedNatively(path), path, spec)
}

// hostMountedPathOn is checkHostMountedPath with the platform and the runtime
// passed in. Linux shares a filesystem with PHP and a natively served site runs
// PHP on the host, so neither crosses a VM boundary and neither gets a row.
func hostMountedPathOn(goos string, native bool, path string, spec config.DoctorCheck) (Check, bool) {
	if goos != "darwin" || native {
		return Check{}, false
	}
	var present []string
	for _, p := range spec.Paths {
		if _, err := os.Stat(filepath.Join(path, p)); err == nil {
			present = append(present, p)
		}
	}
	if len(present) == 0 {
		return Check{}, false
	}
	detail := spec.Detail
	if detail == "" {
		detail = fmt.Sprintf("%s %s on the macOS bind mount, so every read and write there crosses virtiofs. Moving %s onto a path inside the container is worth a large part of the request time.",
			strings.Join(present, ", "), Plural(len(present), "sits", "sit"), Plural(len(present), "it", "them"))
	}
	return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusWarn), Detail: detail, Fix: spec.Fix}, true
}

// siteServedNatively reports whether the host PHP serves this project. An
// unregistered path is on the container runtime by definition.
func siteServedNatively(path string) bool {
	site, err := config.FindSiteByPath(path)
	return err == nil && site != nil && site.IsNative()
}
