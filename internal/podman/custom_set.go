package podman

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
)

// fpmCustomSetHashLabel fingerprints the declared extension/package set an FPM
// image was built from. The Containerfile hash tracks the base recipe only and
// cannot tell that a version's image predates an extension the user has since
// declared, so a second label carries what the first cannot.
const fpmCustomSetHashLabel = "dev.lerd.fpm.custom-set-hash"

// customSetHash fingerprints the declared set. Sorted, so the order entries were
// added in never rebuilds an image; empty for an empty set, so images built
// before this label existed (which read back as "") still count as current for
// the users who declare nothing.
func customSetHash(exts []string, extDeps map[string][]string, packages []string) string {
	if len(exts) == 0 && len(packages) == 0 {
		return ""
	}
	parts := []string{
		strings.Join(sortedCopy(exts), " "),
		strings.Join(sortedCopy(packages), " "),
	}
	deps := make([]string, 0, len(extDeps))
	for ext, d := range extDeps {
		deps = append(deps, ext+"="+strings.Join(sortedCopy(d), ","))
	}
	parts = append(parts, sortedCopy(deps)...)

	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	sort.Strings(out)
	return out
}

// realisedSet reports which declared entries the image actually carries, read
// from its own `php -m` and `apk info`. Never derived from what was declared:
// the build drops what it cannot install by design (`|| true`), so declaring a
// thing is not evidence the image has it.
func realisedSet(declaredExts, declaredPkgs []string, phpMinusM, apkInfo string) config.RealisedPHPSet {
	var set config.RealisedPHPSet
	modules := phpModules(phpMinusM)
	for _, ext := range declaredExts {
		if modules[CanonicalExtension(ext)] {
			set.Extensions = append(set.Extensions, ext)
		}
	}
	installed := apkPackages(apkInfo)
	for _, pkg := range declaredPkgs {
		if installed[pkg] {
			set.Packages = append(set.Packages, pkg)
		}
	}
	return set
}

// VerifyPackagesInstalled checks that the freshly built image actually carries
// each package, by reading its own apk database. The build installs packages
// tolerantly so the legacy Alpine 3.16 images survive a package they do not
// have, which means only this can tell a typo from a success.
func VerifyPackagesInstalled(version string, packages []string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	out, err := execCommand(PodmanBin(), "run", "--rm", FPMImageName(version), "apk", "info").Output()
	if err != nil {
		return nil, fmt.Errorf("inspecting packages in %s: %w", FPMImageName(version), err)
	}
	installed := apkPackages(string(out))
	var missing []string
	for _, p := range packages {
		if !installed[p] {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

// FPMImageExists reports whether a version's FPM image has actually been built.
// This is not the same question as php.ListInstalled, which reads quadlet files:
// writing a quadlet is cheap and several paths do it for a version whose image
// was never built, so only the image itself can answer "can this version serve
// a request".
func FPMImageExists(version string) bool {
	return execCommand(PodmanBin(), "image", "exists", FPMImageName(version)).Run() == nil
}

// FPMImageID returns the local image ID for a version, or "" when it has none.
// Callers use it to cache anything read out of that image: the ID changes on
// every rebuild, so it is an exact invalidation key rather than a TTL guess.
func FPMImageID(version string) string {
	out, err := execCommand(PodmanBin(), "image", "inspect", "--format", "{{.Id}}", FPMImageName(version)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FPMPHPModules returns the raw `php -m` output from a version's image.
func FPMPHPModules(version string) (string, error) {
	out, err := execCommand(PodmanBin(), "run", "--rm", FPMImageName(version), "php", "-m").Output()
	if err != nil {
		return "", fmt.Errorf("reading modules from %s: %w", FPMImageName(version), err)
	}
	return string(out), nil
}

// FPMImageStale reports whether a version's image was built from a different
// declared set than the one configured now, which is the only honest way to
// know that an image predates an extension the user has since declared. An
// image built before this label existed reads back as "", so it is stale for
// anyone who declares anything and current for anyone who declares nothing.
func FPMImageStale(version string) bool {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return false
	}
	want := customSetHash(cfg.GetExtensions(), cfg.AllExtApkDeps(), cfg.GetPackages())
	return imageLabelFn(FPMImageName(version), fpmCustomSetHashLabel) != want
}

// realisedSetMu serializes the read-modify-write of the realised map so the
// per-version builds php:rebuild fires in parallel can't clobber each other.
var realisedSetMu sync.Mutex

// RecordRealisedSet asks a freshly built image what it actually carries and
// records it against the version. Best-effort: a version whose image can't be
// inspected simply has no record, which callers read as "nothing known yet"
// rather than "nothing installed".
func RecordRealisedSet(version string, declaredExts, declaredPkgs []string) {
	if len(declaredExts) == 0 && len(declaredPkgs) == 0 {
		clearRealisedSet(version)
		return
	}
	image := FPMImageName(version)
	modules, err := execCommand(PodmanBin(), "run", "--rm", image, "php", "-m").Output()
	if err != nil {
		return
	}
	// `apk info` needs no network: it reads the image's own installed database.
	apkInfo, err := execCommand(PodmanBin(), "run", "--rm", image, "apk", "info").Output()
	if err != nil {
		return
	}

	realisedSetMu.Lock()
	defer realisedSetMu.Unlock()
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	set := realisedSet(declaredExts, declaredPkgs, string(modules), string(apkInfo))
	set.Hash = customSetHash(declaredExts, cfg.AllExtApkDeps(), declaredPkgs)
	cfg.SetRealised(version, set)
	_ = config.SaveGlobal(cfg)
}

// clearRealisedSet drops a version's record once nothing is declared, so a
// stale record can't outlive the set it described.
func clearRealisedSet(version string) {
	realisedSetMu.Lock()
	defer realisedSetMu.Unlock()
	cfg, err := config.LoadGlobal()
	if err != nil {
		return
	}
	if _, ok := cfg.PHP.Realised[version]; !ok {
		return
	}
	delete(cfg.PHP.Realised, version)
	_ = config.SaveGlobal(cfg)
}

// apkPackages folds `apk info` output into a set. It prints one package name per
// line; `apk info -v` would append versions, so the name-only form is what the
// caller runs and what a user would have typed into `lerd php:pkg add`.
func apkPackages(out string) map[string]bool {
	pkgs := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			pkgs[line] = true
		}
	}
	return pkgs
}
