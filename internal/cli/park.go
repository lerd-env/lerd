package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/linker"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// extMismatch is a composer.json requirement whose extension is in the image but
// under a different platform name, so composer install still fails its check.
type extMismatch struct {
	Required string // the ext-* name composer.json asks for
	Platform string // the ext-* name composer actually publishes
}

// extUnavailable is a requirement no image for this PHP version can satisfy: the
// extension only exists from a later version, so php:ext add can never build it.
type extUnavailable struct {
	Required string // the ext-* name composer.json asks for
	Since    string // the first PHP version that ships it
}

// checkExtensions compares composer's ext-* requirements against the image, folding
// both spellings of an extension onto one name so ext-opcache and ext-zend-opcache
// resolve to the same module. A missing extension that is version-gated is reported
// separately: bundled already excludes it for this version, so it cannot be built.
func checkExtensions(detected, bundled, installed []string) ([]string, []extUnavailable, []extMismatch) {
	inSet := func(ext string, set []string) bool {
		for _, e := range set {
			if podman.CanonicalExtension(e) == ext {
				return true
			}
		}
		return false
	}

	var missing []string
	var unavailable []extUnavailable
	var misnamed []extMismatch
	for _, ext := range detected {
		canonical := podman.CanonicalExtension(ext)
		if !inSet(canonical, bundled) && !inSet(canonical, installed) {
			if since, gated := podman.BundledSince(canonical); gated {
				unavailable = append(unavailable, extUnavailable{Required: ext, Since: since})
				continue
			}
			missing = append(missing, ext)
			continue
		}
		if platform := podman.ComposerPlatformName(canonical); platform != strings.ToLower(ext) {
			misnamed = append(misnamed, extMismatch{Required: ext, Platform: platform})
		}
	}
	return missing, unavailable, misnamed
}

// customExtensionsOn returns the declared extensions this version's image
// actually loaded. The declared set applies to every version, but a version
// cannot always honour it (mongodb needs 8.1+), and treating a declared-but-
// unbuildable extension as present would silence the very warning a site
// requiring it needs. Versions with no recorded build fall back to the declared
// set, which is the most that is known about them.
func customExtensionsOn(cfg *config.GlobalConfig, phpVersion string) []string {
	declared := cfg.GetExtensions()
	missing := cfg.MissingFromImage(phpVersion, declared)
	if len(missing) == 0 {
		return declared
	}
	return slices.DeleteFunc(slices.Clone(declared), func(e string) bool { return slices.Contains(missing, e) })
}

// warnMissingExtensions checks composer.json for ext-* requirements and warns if any are
// not covered by the bundled image or the user's custom extension list.
func warnMissingExtensions(dir, name, phpVersion string, cfg *config.GlobalConfig) {
	detected := phpDet.DetectExtensions(dir)
	if len(detected) == 0 {
		return
	}
	missing, unavailable, misnamed := checkExtensions(detected, podman.BundledExtensions(phpVersion), customExtensionsOn(cfg, phpVersion))

	if len(missing) > 0 {
		fmt.Printf("  [!] %s requires PHP extensions not in the image: %s\n", name, strings.Join(missing, ", "))
		fmt.Printf("      Run: lerd php:ext add %s\n", strings.Join(missing, " "))
	}
	for _, u := range unavailable {
		fmt.Printf("  [!] %s requires ext-%s, which is not available on PHP %s (first shipped on %s)\n", name, u.Required, phpVersion, u.Since)
		fmt.Printf("      lerd php:ext add cannot build it. Move the site to PHP %s or newer, or require a polyfill package instead.\n", u.Since)
	}
	for _, m := range misnamed {
		fmt.Printf("  [!] %s requires ext-%s, which composer publishes as ext-%s\n", name, m.Required, m.Platform)
		fmt.Printf("      The extension is in the image; composer install will still fail its platform check.\n")
		fmt.Printf("      Require ext-%s in composer.json instead.\n", m.Platform)
	}
}

// NewParkCmd returns the park command.
func NewParkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "park [directory]",
		Short: "Park a directory to serve all subdirectories as sites",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPark,
	}
}

func runPark(_ *cobra.Command, args []string) error {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Resolve absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if absDir == "/" {
		return fmt.Errorf("refusing to park the filesystem root; lerd would bind-mount / into every container and shadow its rootfs")
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	// If the target directory is itself a framework project, refuse to park it.
	if _, ok := config.DetectFramework(absDir); ok {
		fmt.Printf("'%s' looks like a framework project, not a directory of projects.\n", absDir)
		fmt.Printf("Run 'lerd link' from that directory instead.\n")
		return nil
	}

	feedback.Begin()
	feedback.Line("parking " + feedback.Val(absDir))

	// Add to parked directories in global config
	found := false
	for _, pd := range cfg.ParkedDirectories {
		if pd == absDir {
			found = true
			break
		}
	}
	if !found {
		cfg.ParkedDirectories = append(cfg.ParkedDirectories, absDir)
		if err := config.SaveGlobal(cfg); err != nil {
			return err
		}
	}

	// Scan subdirectories
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return err
	}
	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, filepath.Join(absDir, entry.Name()))
		}
	}
	if len(projects) == 0 {
		feedback.Line("no subdirectories to scan")
		return nil
	}

	// Each link writes only its own vhost and quadlet; the reloads that publish
	// them are hoisted out of the loop, since they rewrite every quadlet and
	// every container hosts entry and would otherwise make a park quadratic.
	bar := feedback.StartProgress(fmt.Sprintf("linking %d projects", len(projects)), len(projects))
	var registered, skipped []ParkOutcome
	versions := map[string]bool{}
	for _, projectDir := range projects {
		out, err := RegisterProjectDeferred(projectDir, cfg, true)
		switch {
		case err != nil:
			bar.Failed(out.Name, err.Error())
		case out.Registered:
			bar.Step(out.Name)
			registered = append(registered, out)
			versions[out.PHPVersion] = true
		default:
			bar.Skip(out.Name, out.Reason)
			skipped = append(skipped, out)
		}
	}
	bar.Done(parkTally(bar.Completed(), bar.Skipped(), bar.Failures()))

	if len(registered) == 0 {
		if parkAllUnrecognised(skipped) {
			feedback.Line("no PHP projects found in directory")
		}
		// Still refresh the mounts: the parked directory itself is new.
		_ = podman.RewriteFPMQuadlets()
		reportParkSkips(skipped)
		return nil
	}

	publish := feedback.Start("publishing")
	names := make([]string, 0, len(registered))
	for _, r := range registered {
		names = append(names, r.Name)
	}
	phpVersions := make([]string, 0, len(versions))
	for v := range versions {
		phpVersions = append(phpVersions, v)
	}
	if err := siteops.PublishLinks(phpVersions, names...); err != nil {
		publish.Fail(err)
		return err
	}
	publish.OK(fmt.Sprintf("%d site(s) serving", len(registered)))

	reportParkSkips(skipped)

	return nil
}

// parkAllUnrecognised reports whether every skip was simply a directory that is
// not a PHP project, which is the only case worth calling an empty park.
func parkAllUnrecognised(skipped []ParkOutcome) bool {
	for _, sk := range skipped {
		if sk.Reason != reasonNotPHP {
			return false
		}
	}
	return true
}

// reportParkSkips names the directories a park passed over for a reason the
// user can act on. A directory that simply is not a PHP project is left out:
// in a large tree those are the majority and listing them buries the rest.
func reportParkSkips(skipped []ParkOutcome) {
	for _, sk := range skipped {
		if sk.Reason == reasonNotPHP || sk.Reason == "" {
			continue
		}
		if strings.HasPrefix(sk.Reason, "already registered") {
			continue
		}
		feedback.Line(feedback.Dim("skipped " + sk.Name + " — " + sk.Reason))
	}
}

// parkTally is the one-line count a park's progress bar closes with.
func parkTally(linked, skipped, failed int) string {
	out := fmt.Sprintf("%d linked", linked)
	if skipped > 0 {
		out += fmt.Sprintf(", %d skipped", skipped)
	}
	if failed > 0 {
		out += fmt.Sprintf(", %d failed", failed)
	}
	return out
}

// reasonNotPHP marks a directory that does not look like a PHP project at all.
const reasonNotPHP = "not a PHP project"

// ParkOutcome says what registering one project did, so a batch can tally the
// results and report a reason for everything it passed over.
type ParkOutcome struct {
	Name       string
	Registered bool
	Skipped    bool
	Reason     string
	PHPVersion string
	Detail     string
}

// RegisterProject registers a single project directory as a site and publishes
// it immediately. The watcher uses it for a project that appeared on its own;
// `lerd park` uses RegisterProjectDeferred so a whole directory publishes once.
func RegisterProject(projectDir string, cfg *config.GlobalConfig) (bool, error) {
	out, err := RegisterProjectDeferred(projectDir, cfg, false)
	return out.Registered, err
}

// RegisterProjectDeferred registers one project under the watcher's policy: it
// reads the project's committed configuration but asks nothing, writes nothing
// into the project, and runs nothing the repository authored. With defer set,
// the reloads that publish the link are left to siteops.PublishLinks so a batch
// pays for them once.
func RegisterProjectDeferred(projectDir string, cfg *config.GlobalConfig, defer_ bool) (ParkOutcome, error) {
	name := filepath.Base(projectDir)
	out := ParkOutcome{Name: name}

	// Don't register a directory that lives inside an existing framework project.
	// This prevents Laravel subdirs (app/, vendor/, public/, etc.) from being
	// registered as sites when a project root is accidentally used as a park dir.
	if _, ok := config.DetectFramework(filepath.Dir(projectDir)); ok {
		out.Skipped, out.Reason = true, "inside a framework project; run 'lerd link' in "+filepath.Dir(projectDir)
		return out, nil
	}
	if !parkAdmits(projectDir) {
		out.Skipped, out.Reason = true, reasonNotPHP
		return out, nil
	}

	policy := linker.WatcherPolicy()
	policy.DeferPublish = defer_
	plan, err := linker.Resolve(projectDir, cfg, policy)
	if err != nil {
		return out, err
	}
	if !plan.Registered() {
		out.Skipped, out.Reason = true, plan.SkipDetail
		return out, nil
	}
	// A project that serves itself from a container, proxies a dev server, or
	// asks for a per-site runtime needs an image built or a repository command
	// run. Neither belongs in an unattended sweep, so it is left for an explicit
	// link rather than silently downgraded to plain FPM.
	if plan.Mode != linker.ModeFPM {
		out.Skipped, out.Reason = true, "declares its own "+string(plan.Mode)+" runtime; run 'lerd link' in it"
		return out, nil
	}
	if linker.IsReservedDomain(plan.Site.PrimaryDomain()) {
		out.Skipped, out.Reason = true, "domain is reserved"
		return out, nil
	}

	warnMissingExtensions(projectDir, plan.Site.Name, plan.Site.PHPVersion, cfg)

	res, err := linker.Apply(plan, policy, linker.Deps{}, linker.NopReporter{})
	if err != nil {
		return out, err
	}

	label := plan.FrameworkLabel
	out.Registered = true
	out.Name = res.Site.Name
	out.PHPVersion = res.Site.PHPVersion
	out.Detail = strings.Join(res.Site.Domains, ", ") + " · php " + res.Site.PHPVersion + " · " + label
	return out, nil
}

// parkAdmits reports whether a directory looks enough like a PHP project to
// register unattended: a framework lerd knows, a real document root, or a
// composer.json / top-level PHP file.
func parkAdmits(projectDir string) bool {
	if _, ok := config.DetectFrameworkForDir(projectDir); ok {
		return true
	}
	if config.DetectPublicDir(projectDir) != "." {
		return true
	}
	return looksLikePHPProject(projectDir)
}

// looksLikePHPProject returns true if dir contains composer.json or any .php file
// at the top level, indicating it is likely a PHP project worth registering.
func looksLikePHPProject(dir string) bool {
	return phpDet.IsPHPProject(dir)
}

// ensureFPMQuadlet builds the PHP image if needed, then writes (or overwrites) the quadlet.
func ensureFPMQuadlet(phpVersion string) error {
	return ensureFPMQuadletTo(phpVersion, os.Stdout)
}

// Seams for the ensure path, swappable in tests. Every step shells out to
// podman or writes real state, so a test of the start/restart decision alone
// would otherwise build a container storage tree to reach it.
var (
	writeFPMQuadlet = podman.WriteFPMQuadlet
	buildFPMImageTo = podman.BuildFPMImageTo
	ensureXdebugIni = podman.EnsureXdebugIni
	startUnitFn     = podman.StartUnit
	restartUnitFn   = podman.RestartUnit
)

// ensureFPMQuadletTo is like ensureFPMQuadlet but writes build output to w.
func ensureFPMQuadletTo(phpVersion string, w io.Writer) error {
	versionShort := strings.ReplaceAll(phpVersion, ".", "")
	unitName := "lerd-php" + versionShort + "-fpm"

	// Write the unit file first so the version is registered in lerd status even
	// if the image build fails — lerd start will rebuild the image on the next run.
	if err := writeFPMQuadlet(phpVersion); err != nil {
		return err
	}

	rebuilt, err := buildFPMImageTo(phpVersion, false, w)
	if err != nil {
		return fmt.Errorf("building FPM image for PHP %s: %w", phpVersion, err)
	}

	_ = ensureXdebugIni(phpVersion)

	// A start is a no-op on an already-active unit, so a version whose image was
	// just rebuilt here (the deferred half of a php:ext / php:pkg change) would
	// keep serving the old image while every status surface reported the new set.
	if rebuilt {
		return restartUnitFn(unitName)
	}
	return startUnitFn(unitName)
}
