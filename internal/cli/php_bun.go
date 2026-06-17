package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpBunCmd returns the php:bun parent command, which manages an optional
// in-container bun runtime. lerd never pins or version-manages bun: install
// pulls the latest musl build into a persistent volume via the bundled npm,
// and `bun upgrade` (run inside `lerd shell`) self-updates from there.
func NewPhpBunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:bun",
		Short: "Manage an optional bun runtime inside the PHP-FPM container",
	}
	cmd.AddCommand(newPhpBunInstallCmd())
	cmd.AddCommand(newPhpBunRemoveCmd())
	cmd.AddCommand(newPhpBunUpdateCmd())
	cmd.AddCommand(newPhpBunVersionCmd())
	return cmd
}

func newPhpBunUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [php-version]",
		Short: "Update the container's bun in place (bun upgrade)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := bunDisplayVersion(args)
			if err != nil {
				return err
			}
			container := bunFPMContainer(version)
			if running, _ := podman.ContainerRunning(container); !running {
				return fmt.Errorf("PHP %s FPM container is not running — start it with: %s", version, serviceStartHint(container))
			}
			if !bunInstalledInContainer(version) {
				return fmt.Errorf("bun is not installed in the PHP %s container — run: lerd php:bun install", version)
			}
			fmt.Printf("Updating bun in the PHP %s container...\n", version)
			up := podman.Cmd("exec", container, "/root/.bun/bin/bun", "upgrade")
			up.Stdout = os.Stdout
			up.Stderr = os.Stderr
			if err := up.Run(); err != nil {
				return fmt.Errorf("bun upgrade: %w", err)
			}
			out, _ := podman.Cmd("exec", container, "/root/.bun/bin/bun", "--version").CombinedOutput()
			fmt.Printf("bun is now %s in the PHP %s container.\n", strings.TrimSpace(string(out)), version)
			return nil
		},
	}
}

func newPhpBunInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [php-version]",
		Short: "Install (or update) bun inside the PHP-FPM container",
		Long: "Installs a musl bun into the container's persistent /root/.bun volume using the bundled npm, so it survives image rebuilds and is shared across every PHP version. " +
			"Run `bun upgrade` inside `lerd shell` to update it later; lerd does not pin a version.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := phpExtVersion(args)
			if err != nil {
				return err
			}
			pin, _ := cmd.Flags().GetString("pin")
			return installContainerBun(version, pin, os.Stdout)
		},
	}
	cmd.Flags().String("pin", "", "pin a specific bun version (e.g. 1.1.45) instead of latest")
	return cmd
}

func newPhpBunRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the in-container bun and clear its persistent volume",
		Long: "Deletes the musl bun installed by `php:bun install` from the shared /root/.bun volume. " +
			"bun lives in one host-backed volume shared across every PHP version, so this removes it everywhere at once; the container need not be running.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return removeContainerBun(os.Stdout)
		},
	}
}

// removeContainerBun clears the shared host-backed /root/.bun volume, deleting
// the musl bun installed by `php:bun install`. The volume is shared across every
// PHP version, so this removes bun everywhere at once and needs no container.
func removeContainerBun(w io.Writer) error {
	dir := podman.BunVolumeDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading bun volume: %w", err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(w, "bun is not installed in the PHP-FPM container; nothing to remove.")
		return nil
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("removing %s from the bun volume: %w", e.Name(), err)
		}
	}
	fmt.Fprintln(w, "Removed the in-container bun and cleared its volume. Reinstall with `lerd php:bun install`.")
	return nil
}

// fpmContainerName returns the FPM container/unit name for a PHP version.
func fpmContainerName(version string) string {
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}

// bunFPMSite returns the custom-FPM site the current directory belongs to, or nil
// when the cwd isn't inside one (the common shared-container case).
func bunFPMSite() *config.Site {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	s, err := config.FindSiteByPath(siteRootFor(cwd))
	if err != nil || s == nil || !s.IsCustomFPM() {
		return nil
	}
	return s
}

// bunFPMContainer resolves the FPM container php:bun should exec into for version:
// the per-site custom-FPM container when run inside a custom-FPM site, otherwise
// the shared lerd-php<version>-fpm container. bun lives in a host-backed volume
// every FPM container shares, so a custom-FPM-only user can reach it through their
// own running container instead of being told to start a shared one they don't use.
func bunFPMContainer(version string) string {
	if s := bunFPMSite(); s != nil {
		return podman.CustomFPMContainerName(s.Name)
	}
	return fpmContainerName(version)
}

// bunInstalledInContainer reports whether a working bun already lives in the
// resolved FPM container volume.
func bunInstalledInContainer(version string) bool {
	container := bunFPMContainer(version)
	return podman.Cmd("exec", container, "/root/.bun/bin/bun", "--version").Run() == nil
}

// bunDisplayVersion is the PHP version php:bun reports in its messages: a
// custom-FPM site's pinned version (fixed by its Containerfile FROM line, which can
// differ from what the project would detect), otherwise the version from args or
// cwd detection. Keeps the reported version aligned with the container actually used.
func bunDisplayVersion(args []string) (string, error) {
	version, err := phpExtVersion(args)
	if err != nil {
		return "", err
	}
	if s := bunFPMSite(); s != nil {
		return s.PHPVersion, nil
	}
	return version, nil
}

// bunVolumeMounted reports whether the persistent /root/.bun volume is already
// mounted in the running container, so we only restart it when the mount is
// genuinely missing (e.g. a container created before this feature shipped).
func bunVolumeMounted(container string) bool {
	return fpmVolumeMounted(container, "/root/.bun")
}

// fpmVolumeMounted reports whether mountPath is a live mount inside the running
// container. Shared by the bun and Pest-browser volume probes so the /proc/mounts
// matching logic lives in one place.
func fpmVolumeMounted(container, mountPath string) bool {
	return podman.Cmd("exec", container, "sh", "-c", "grep -qF ' "+mountPath+" ' /proc/mounts").Run() == nil
}

// restartFPMAndWait restarts a PHP-FPM unit and blocks until it reports running,
// so an exec right after the restart doesn't race the container.
func restartFPMAndWait(container string) error {
	if err := podman.RestartUnit(container); err != nil {
		return fmt.Errorf("restarting %s: %w", container, err)
	}
	return waitContainerRunning(container, 20*time.Second)
}

// installContainerBun installs (or reinstalls) a musl bun into the version's
// FPM container, using the bundled npm so npm's platform detection pulls the
// musl build. It only restarts the container when the persistent volume isn't
// mounted yet, so reruns on an already-prepared container are non-disruptive.
func installContainerBun(version, pin string, w io.Writer) error {
	// Target the per-site custom-FPM container when run inside one, otherwise the
	// shared per-version container. Either way we rewrite the matching quadlet so a
	// container created before the bun volume shipped gains the /root/.bun mount.
	site := bunFPMSite()
	container := fpmContainerName(version)
	if site != nil {
		container = podman.CustomFPMContainerName(site.Name)
		// A custom-FPM site's version is pinned by its Containerfile, so report that
		// rather than the detected one in the messages below and the quadlet.
		version = site.PHPVersion
		if err := podman.WriteCustomFPMQuadlet(site.Name, version); err != nil {
			return fmt.Errorf("updating custom FPM quadlet: %w", err)
		}
	} else if err := podman.WriteFPMQuadlet(version); err != nil {
		return fmt.Errorf("updating FPM quadlet: %w", err)
	}
	if running, _ := podman.ContainerRunning(container); !running {
		return fmt.Errorf("PHP %s FPM container is not running — start it with: %s", version, serviceStartHint(container))
	}
	if !bunVolumeMounted(container) {
		fmt.Fprintf(w, "Preparing PHP %s container for bun...\n", version)
		if err := restartFPMAndWait(container); err != nil {
			return err
		}
	}

	pkg := "bun"
	if pin != "" {
		pkg = "bun@" + pin
	}
	fmt.Fprintf(w, "Installing %s into the PHP %s container (musl build via npm)...\n", pkg, version)
	install := podman.Cmd("exec", container, "npm", "install", "-g", "--prefix", "/root/.bun", pkg)
	install.Stdout = w
	install.Stderr = w
	if err := install.Run(); err != nil {
		return fmt.Errorf("installing bun in container: %w", err)
	}

	out, err := podman.Cmd("exec", container, "/root/.bun/bin/bun", "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("bun did not run in the container after install: %w\n%s", err, out)
	}
	fmt.Fprintf(w, "bun %s installed in PHP %s container. Use it from `lerd shell`; update it with `bun upgrade`.\n", strings.TrimSpace(string(out)), version)
	return nil
}

func newPhpBunVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version [php-version]",
		Short: "Show the bun version installed in the PHP-FPM container",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := bunDisplayVersion(args)
			if err != nil {
				return err
			}
			container := bunFPMContainer(version)
			if running, _ := podman.ContainerRunning(container); !running {
				return fmt.Errorf("PHP %s FPM container is not running — start it with: %s", version, serviceStartHint(container))
			}
			out, err := podman.Cmd("exec", container, "/root/.bun/bin/bun", "--version").CombinedOutput()
			if err != nil {
				fmt.Printf("bun is not installed in the PHP %s container — run: lerd php:bun install\n", version)
				return nil
			}
			fmt.Printf("bun %s (PHP %s container)\n", strings.TrimSpace(string(out)), version)
			return nil
		},
	}
}

// waitContainerRunning polls until the named container reports running or the
// timeout elapses, so an exec right after a restart doesn't race the container.
func waitContainerRunning(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if running, _ := podman.ContainerRunning(name); running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to start", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
