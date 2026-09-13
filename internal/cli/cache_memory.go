package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewCacheMemoryCmd returns the cache:memory command. It flips cache_in_memory
// in the current site's .lerd.yaml, which mounts the paths the framework
// declares as tmpfs inside the PHP container so the compiled cache never
// crosses the macOS bind mount.
func NewCacheMemoryCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "cache:memory [on|off]",
		Short: "Keep the framework's compiled cache in memory instead of on the macOS mount",
		Long: "Mounts the directories the site's framework declares (Symfony's var/cache) as tmpfs inside the PHP container, " +
			"so the compiled cache is written to memory rather than across the virtiofs bind mount. " +
			"The cache dies with the container, the host sees an empty directory where it used to be, and the shared PHP-FPM container restarts for every site on this PHP version. " +
			"macOS on the container runtime only. Run with no argument to show the current setting.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			site, err := resolveSiteForCwd()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return showCacheInMemory(*site, os.Stdout)
			}
			on, ok := onOffValue(args[0])
			if !ok {
				return fmt.Errorf("unknown setting %q — use on or off", args[0])
			}
			return applyCacheInMemory(*site, on, yes, os.Stdout)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Restart the shared PHP container without prompting")
	return cmd
}

func onOffValue(choice string) (on, ok bool) {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "on", "true", "yes":
		return true, true
	case "off", "false", "no":
		return false, true
	}
	return false, false
}

func showCacheInMemory(site config.Site, w io.Writer) error {
	proj, err := config.LoadProjectConfig(site.Path)
	state := "off"
	if err == nil && proj != nil && proj.CacheInMemory {
		state = "on"
	}
	fmt.Fprintf(w, "In-memory cache for %s: %s\n", site.Name, state)
	if paths := config.TmpfsPathsForDir(site.Path); len(paths) > 0 {
		fmt.Fprintf(w, "Declared paths: %s\n", strings.Join(paths, ", "))
	} else {
		fmt.Fprintln(w, "This site's framework declares no cache path to hold in memory.")
	}
	return nil
}

// applyCacheInMemory persists the choice and restarts the shared PHP container
// so the mount takes effect. The restart hits every site on this PHP version,
// which is why it confirms first.
func applyCacheInMemory(site config.Site, on, yes bool, w io.Writer) error {
	if err := SetCacheInMemory(site, on); err != nil {
		return err
	}
	state := "off"
	if on {
		state = "on"
	}
	fmt.Fprintf(w, "In-memory cache for %s set to %s.\n", site.Name, state)
	if !yes && !feedback.Confirm(fmt.Sprintf("Restart the shared PHP %s container now? Every site on that version restarts with it.", site.PHPVersion), true) {
		fmt.Fprintln(w, "Saved. The mount applies the next time the PHP container restarts.")
		return nil
	}
	return podman.RewriteFPMQuadlets()
}

// SetCacheInMemory records the choice for a site without touching any container,
// refusing the platforms where a tmpfs buys nothing. Shared with the dashboard's
// doctor fix, which applies it and then rewrites the shared unit itself.
func SetCacheInMemory(site config.Site, on bool) error {
	if on && runtime.GOOS != "darwin" {
		return fmt.Errorf("in-memory cache is a macOS feature: on Linux the project and PHP already share a filesystem, so there is no mount to escape")
	}
	if on && nativeRuntimeActive() {
		return fmt.Errorf("in-memory cache needs the container runtime: the native runtime runs PHP on the host, which never crosses the mount")
	}
	return setCacheInMemory(site.Path, on)
}

// EnableCacheInMemory is the doctor fix behind the check that reports a cache
// sitting on the macOS mount: it records the opt-in and rewrites the shared PHP
// unit so the tmpfs is there on the next request.
func EnableCacheInMemory(site config.Site) error {
	if err := SetCacheInMemory(site, true); err != nil {
		return err
	}
	return podman.RewriteFPMQuadlets()
}

// setCacheInMemory writes the opt-in to the untracked .lerd.local.yaml rather
// than the committed .lerd.yaml: this is a macOS-only trade-off about one
// machine's filesystem, so a teammate on Linux should not inherit it from git.
// Turning it on is refused when the framework names no path, which would mount
// nothing and look like a no-op; turning it off is always allowed.
func setCacheInMemory(dir string, on bool) error {
	if on && len(config.TmpfsPathsForDir(dir)) == 0 {
		return fmt.Errorf("this site's framework declares no cache path to hold in memory")
	}
	if err := config.SetLocalOverride(dir, "cache_in_memory", on); err != nil {
		return err
	}
	addToGitignore(dir, config.LocalOverrideFile)
	return nil
}

func nativeRuntimeActive() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative
}
