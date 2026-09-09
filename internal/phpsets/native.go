package phpsets

import (
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
)

// Seams for the host build, so what is reported can be tested without one.
var (
	nativeRuntimeFn = func() bool {
		cfg, err := config.LoadGlobal()
		return err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative
	}
	nativeBuiltFn = func(version string) bool {
		_, err := os.Stat(nativephp.BinaryPath(version))
		return err == nil
	}
	nativeModulesFn = nativephp.Extensions
)

// nativeVersionStatus measures the declared sets against the host build rather
// than an image. The build's extensions are fixed when it is compiled, so
// nothing here can be stale or want rebuilding; either an entry is in the
// binary or it is not.
func nativeVersionStatus(r Report, version string) Report {
	if !nativeBuiltFn(version) {
		return r
	}
	r.Built = true
	mods, err := nativeModulesFn(version)
	if err != nil {
		// The binary is there but would not answer. Saying nothing is present
		// would read as a build missing everything, which it is not.
		return r
	}
	present := map[string]bool{}
	for _, m := range mods {
		present[canonicalModule(m)] = true
	}
	r.Extensions.Has, r.Extensions.Cannot = partition(r.Extensions.Declared, present)
	// Packages are Alpine packages installed into an image. There is no image
	// here, so none of them are present and none can be, which the tab says in
	// its own words rather than listing them as failures.
	return r
}

// canonicalModule folds what PHP prints to what lerd declares: it reports
// OPcache as "Zend OPcache", and comparing literally would call a compiled-in
// extension missing.
func canonicalModule(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "zend ")
	return strings.ReplaceAll(n, " ", "")
}

// partition splits declared entries into those the build carries and those it
// does not.
func partition(declared []string, present map[string]bool) (has, cannot []string) {
	for _, d := range declared {
		if present[canonicalModule(d)] {
			has = append(has, d)
			continue
		}
		cannot = append(cannot, d)
	}
	return has, cannot
}
