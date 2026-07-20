package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// pestBrowserPkg is the Alpine package baked into the shared FPM image so
// Playwright has a musl-native browser to drive. Playwright's own downloaded
// Chromium is a glibc binary and cannot run on Alpine's musl libc.
const pestBrowserPkg = "chromium"

// pestBrowserCachePath is where the Playwright registry and lerd's chromium
// shims live: a persistent volume, baked into the image as PLAYWRIGHT_BROWSERS_PATH
// (see podman.PlaywrightCachePath / buildCustomPackagesBlock) so `lerd test`/
// `lerd pest` find the browsers regardless of the host HOME they exec with.
const pestBrowserCachePath = podman.PlaywrightCachePath

// playwrightBinRel is the project-relative path to the locally installed
// Playwright CLI, used both to download the registry and to fail fast before any
// expensive work when the npm package is missing.
const playwrightBinRel = "node_modules/.bin/playwright"

// pestBrowserShim rewrites Playwright's downloaded (glibc) browser binaries to a
// wrapper that execs the system musl chromium. Pest hardcodes its launch options
// and exposes no executablePath hook, so the wrapper is the only lever. It
// injects --no-sandbox (required for chromium as root in a container, which Pest
// never passes) and forces HOME=/root: `lerd test`/`lerd pest` exec with the
// host HOME, and musl chromium crashes writing its config into the bind-mounted
// host home, so the browser needs an isolated, writable home. The find globs the
// cache (an undocumented Playwright layout, but the only handle Pest leaves us),
// so it stays correct across browser revisions; if a future Playwright renames
// the binaries the install fails loudly via `test "$n" -gt 0`.
var pestBrowserShim = fmt.Sprintf(`set -e
cache="${PLAYWRIGHT_BROWSERS_PATH:-%s}"
if [ ! -d "$cache" ]; then echo "no Playwright browser cache at $cache" >&2; exit 1; fi
find "$cache" -type f \( -name chrome-headless-shell -o -name chrome \) -print0 2>/dev/null | while IFS= read -r -d '' b; do
  printf '#!/bin/sh\nexport HOME=/root\nexec /usr/bin/chromium --no-sandbox "$@"\n' > "$b"
  chmod +x "$b"
done
n=$(find "$cache" -type f \( -name chrome-headless-shell -o -name chrome \) 2>/dev/null | wc -l)
echo "  shimmed $n browser binary(ies) to system chromium"
test "$n" -gt 0
`, pestBrowserCachePath)

// pestBrowserPlanAwk turns `playwright install --dry-run` output into one
// "<install dir>\t<download url>" line per component, deduplicated (ffmpeg is
// listed once per browser). The dry run is the only supported way to learn the
// exact cache layout and archive URLs for the project's pinned Playwright.
const pestBrowserPlanAwk = `/Install location:/ { dir = $NF }
/Download url:/ { if (dir != "" && !seen[dir]++) print dir "\t" $NF; dir = "" }`

// pestBrowserInstall populates the browser registry from the project's own
// Playwright CLI, but does the fetching itself: it asks Playwright where each
// component belongs and what to download, then uses curl and the image's unzip.
// Playwright's Node extractor deadlocks mid-write on the bind-mounted cache
// (#1006), stalling forever with no output, while unzip writes the same archive
// to the same path in seconds. curl's --speed-time guard turns a stalled
// download into an error instead of another silent hang, and the
// INSTALLATION_COMPLETE marker is what makes Playwright's registry accept the
// result as a real install.
var pestBrowserInstall = fmt.Sprintf(`set -e
pw=./node_modules/.bin/playwright
if [ ! -x "$pw" ]; then
  echo "the 'playwright' npm package is not in node_modules — run: lerd npm install playwright" >&2
  exit 1
fi
cache="${PLAYWRIGHT_BROWSERS_PATH:-%s}"
mkdir -p "$cache"

plan=$("$pw" install --dry-run chromium | awk '%s')
if [ -z "$plan" ]; then
  echo "could not read Playwright's install plan; falling back to its own installer" >&2
  exec "$pw" install chromium
fi

printf '%%s\n' "$plan" | while IFS="$(printf '\t')" read -r dir url; do
  name=$(basename "$dir")
  if [ -f "$dir/INSTALLATION_COMPLETE" ]; then
    echo "  $name is already installed"
    continue
  fi
  zip="/tmp/lerd-playwright-$name.zip"
  echo "  downloading $name..."
  rm -rf "$dir" "$zip"
  curl -fsSL --retry 3 --speed-limit 1024 --speed-time 60 -o "$zip" "$url"
  echo "  extracting $name..."
  mkdir -p "$dir"
  unzip -q -o "$zip" -d "$dir"
  rm -f "$zip"
  find "$dir" -type f \( -name 'chrome' -o -name 'chrome-headless-shell' -o -name 'chrome_sandbox' \
    -o -name 'chrome_crashpad_handler' -o -name 'ffmpeg-linux' -o -name '*.sh' \) -exec chmod +x {} +
  touch "$dir/INSTALLATION_COMPLETE"
done
`, pestBrowserCachePath, pestBrowserPlanAwk)

// pestBrowserCleanup clears what an aborted install leaves inside the container.
// Ctrl+C kills the host-side lerd process only, so the in-container installer
// survives holding Playwright's __dirlock and every retry then blocks on it with
// no output at all. Half-downloaded archives are dropped here too, since nothing
// else ever reclaims them. The bracketed characters keep each pattern from
// matching this script's own command line, which would kill the cleanup itself.
var pestBrowserCleanup = fmt.Sprintf(`pkill -f "oopDownloadBrowserMai[n]" >/dev/null 2>&1
pkill -f "playwright[ ]install" >/dev/null 2>&1
rm -rf "${PLAYWRIGHT_BROWSERS_PATH:-%s}/__dirlock" /tmp/playwright-download-* /tmp/lerd-playwright-*.zip
exit 0
`, pestBrowserCachePath)

// pestBrowserSupportedVersion rejects the frozen legacy PHP tier, whose base
// image ships a Node too old for current Playwright (see docs). Returning early
// here avoids a multi-minute image rebuild that would only fail later.
func pestBrowserSupportedVersion(version string) error {
	if IsLegacyPHPVersion(version) {
		return fmt.Errorf("Pest browser testing is not supported on the legacy PHP %s tier (its bundled Node is too old for Playwright); use a current PHP version", version)
	}
	return nil
}

// NewPestBrowserCmd returns the pest:browser parent command, which sets up
// in-container Pest browser testing. Pest drives Playwright locally where the
// tests run, so the browser must live in the PHP-FPM container. This bakes
// Alpine's musl chromium into the shared image (opt-in, like php:pkg) and shims
// Playwright's glibc browser to it, so no per-site Containerfile is needed.
func NewPestBrowserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pest:browser",
		Short: "Set up in-container Pest browser testing (Playwright on musl chromium)",
	}
	cmd.AddCommand(newPestBrowserInstallCmd())
	cmd.AddCommand(newPestBrowserRemoveCmd())
	cmd.AddCommand(newPestBrowserDoctorCmd())
	return cmd
}

func newPestBrowserInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install [php-version]",
		Short: "Bake musl chromium into the FPM image and wire up Playwright for Pest",
		Long: "Adds chromium to the shared PHP-FPM image (the same package mechanism as php:pkg), rebuilds it, " +
			"downloads the Playwright browser registry into a persistent volume, and shims Playwright's glibc " +
			"browser to the musl chromium so `pestphp/pest-plugin-browser` runs inside the normal FPM container. " +
			"Re-run it after bumping the Playwright version in your project.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := phpExtVersion(args)
			if err != nil {
				return err
			}
			return installPestBrowser(version, os.Stdout)
		},
	}
}

// playwrightVolumeMounted reports whether the persistent Playwright cache volume
// is already mounted in the running container, so reruns only restart it when
// the mount is genuinely missing (a container created before this shipped).
func playwrightVolumeMounted(container string) bool {
	return fpmVolumeMounted(container, pestBrowserCachePath)
}

// playwrightEnvBaked reports whether the image carries PLAYWRIGHT_BROWSERS_PATH,
// which `lerd test`/`lerd pest` rely on to find the browsers in the volume
// regardless of the host HOME they exec with.
func playwrightEnvBaked(container string) bool {
	out, err := podman.Cmd("exec", container, "printenv", "PLAYWRIGHT_BROWSERS_PATH").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func installPestBrowser(version string, w io.Writer) error {
	if err := pestBrowserSupportedVersion(version); err != nil {
		return err
	}
	container := fpmContainerName(version)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Setting up Pest browser testing for PHP %s...\n", version)

	if !config.ComposerHasPackage(cwd, "pestphp/pest-plugin-browser") {
		fmt.Fprintln(w, "  [note] pestphp/pest-plugin-browser is not in composer.json yet.")
		fmt.Fprintln(w, "         Add it with: lerd composer require --dev pestphp/pest-plugin-browser")
	}

	// Fail fast before mutating config or paying for a rebuild: the registry
	// download below needs the project's Playwright CLI.
	if _, statErr := os.Stat(filepath.Join(cwd, playwrightBinRel)); statErr != nil {
		return fmt.Errorf("the playwright npm package is not installed in this project — run `lerd npm install playwright` first")
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	added := false
	if !slices.Contains(cfg.GetPackages(), pestBrowserPkg) {
		cfg.AddPackage(pestBrowserPkg)
		if err := config.SaveGlobal(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		added = true
	}

	if err := podman.WriteFPMQuadlet(version); err != nil {
		return fmt.Errorf("updating FPM quadlet: %w", err)
	}

	if err := ensureFPMStarted(version, container); err != nil {
		return err
	}

	// Rebuild when chromium was just opted in, or when an image from an older
	// install lacks the baked PLAYWRIGHT_BROWSERS_PATH the test runner relies on.
	needRebuild := added || !playwrightEnvBaked(container)
	if needRebuild {
		fmt.Fprintf(w, "Baking chromium into the PHP %s image...\n", version)
		if err := podman.RebuildFPMImageTo(version, false, w); err != nil {
			if added {
				cfg.RemovePackage(pestBrowserPkg)
				_ = config.SaveGlobal(cfg)
			}
			return fmt.Errorf("rebuild failed: %w", err)
		}
		// chromium is an ordinary entry in the declared package set, so it has
		// to reach Octane sites too. This step restarts the FPM container
		// itself below, which is why only the FrankenPHP half runs here.
		rebuildFrankenPHPForVersion(version)
	}
	if needRebuild || !playwrightVolumeMounted(container) {
		fmt.Fprintf(w, "Restarting the PHP %s container...\n", version)
		if err := restartFPMAndWait(container); err != nil {
			return err
		}
	}

	out, err := podman.Cmd("exec", container, "chromium", "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("chromium did not run in the PHP %s container after rebuild: %w\n%s", version, err, out)
	}
	fmt.Fprintf(w, "  %s\n", strings.TrimSpace(string(out)))

	browsersEnv := "PLAYWRIGHT_BROWSERS_PATH=" + pestBrowserCachePath
	fmt.Fprintln(w, "Downloading the Playwright browser registry...")
	if err := runPestBrowserInstall(container, cwd, browsersEnv, w); err != nil {
		return fmt.Errorf("playwright install: %w", err)
	}

	shim := podman.Cmd("exec", "--env", browsersEnv, container, "sh", "-c", pestBrowserShim)
	shim.Stdout = w
	shim.Stderr = w
	if err := shim.Run(); err != nil {
		return fmt.Errorf("shimming Playwright browsers to system chromium: %w", err)
	}

	fmt.Fprintf(w, "\nPest browser testing is ready for PHP %s. Run your suite with `lerd test` or `lerd pest`.\n", version)
	return nil
}

// runPestBrowserInstall fetches the browser registry into the container, running
// the cleanup both before (a previous abort may still hold the install lock) and
// after a failed or interrupted run, so the state a user retries from is always
// recoverable rather than silently wedged.
func runPestBrowserInstall(container, cwd, browsersEnv string, w io.Writer) error {
	cleanup := func() {
		_ = podman.Cmd("exec", "--env", browsersEnv, container, "sh", "-c", pestBrowserCleanup).Run()
	}
	cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	inst := podman.CmdContext(ctx, "exec", "-w", cwd, "--env", browsersEnv, container, "sh", "-c", pestBrowserInstall)
	inst.Stdout = w
	inst.Stderr = w
	err := inst.Run()
	if err != nil {
		cleanup()
	}
	if ctx.Err() != nil {
		return fmt.Errorf("interrupted; the in-container download was stopped and its lock cleared")
	}
	return err
}

func newPestBrowserRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [php-version]",
		Short: "Remove chromium from the FPM image and disable Pest browser testing",
		Long: "Drops chromium from the shared PHP-FPM image and rebuilds it, which also removes the baked " +
			"PLAYWRIGHT_BROWSERS_PATH. The persistent Playwright cache volume is left intact (it may be shared " +
			"with other PHP versions); delete it manually to reclaim disk.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := phpExtVersion(args)
			if err != nil {
				return err
			}
			return removePestBrowser(version, os.Stdout)
		},
	}
}

func removePestBrowser(version string, w io.Writer) error {
	container := fpmContainerName(version)

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if !slices.Contains(cfg.GetPackages(), pestBrowserPkg) {
		fmt.Fprintf(w, "Pest browser testing is not enabled for PHP %s — nothing to remove.\n", version)
		return nil
	}
	cfg.RemovePackage(pestBrowserPkg)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(w, "Removing chromium from the PHP %s image...\n", version)
	if err := podman.RebuildFPMImageTo(version, false, w); err != nil {
		// Restore config so it doesn't claim chromium is gone while the live
		// image still carries it (mirrors installPestBrowser's revert).
		cfg.AddPackage(pestBrowserPkg)
		_ = config.SaveGlobal(cfg)
		return fmt.Errorf("rebuild failed (config restored): %w", err)
	}
	if running, _ := podman.ContainerRunning(container); running {
		if err := restartFPMAndWait(container); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "Pest browser testing removed for PHP %s. The Playwright cache volume is left intact; delete %s to reclaim disk.\n", version, podman.PlaywrightVolumeDir())
	return nil
}

func newPestBrowserDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [php-version]",
		Short: "Diagnose the Pest browser testing setup for a PHP version",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := phpExtVersion(args)
			if err != nil {
				return err
			}
			return doctorPestBrowser(version, os.Stdout)
		},
	}
}

func doctorPestBrowser(version string, w io.Writer) error {
	container := fpmContainerName(version)
	cwd, _ := os.Getwd()

	check := func(ok bool, label, fix string) {
		mark := "x"
		if ok {
			mark = "✓"
		}
		fmt.Fprintf(w, "  [%s] %s\n", mark, label)
		if !ok && fix != "" {
			fmt.Fprintf(w, "        → %s\n", fix)
		}
	}

	fmt.Fprintf(w, "Pest browser testing — PHP %s:\n", version)

	check(config.ComposerHasPackage(cwd, "pestphp/pest-plugin-browser"),
		"pestphp/pest-plugin-browser in composer.json",
		"lerd composer require --dev pestphp/pest-plugin-browser")

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	check(slices.Contains(cfg.GetPackages(), pestBrowserPkg),
		"chromium baked into the FPM image", "lerd pest:browser install")

	running, _ := podman.ContainerRunning(container)
	check(running, "PHP "+version+" FPM container is running", serviceStartHint(container))
	if !running {
		return nil
	}

	chromiumOK := podman.Cmd("exec", container, "chromium", "--version").Run() == nil
	check(chromiumOK, "chromium present in the container", "lerd pest:browser install")

	playwrightOK := podman.Cmd("exec", "-w", cwd, container, "sh", "-c", "test -x ./node_modules/.bin/playwright").Run() == nil
	check(playwrightOK, "playwright npm package installed", "lerd npm install playwright")

	shimOK := podman.Cmd("exec", container, "sh", "-c",
		`fs=$(find "${PLAYWRIGHT_BROWSERS_PATH:-`+pestBrowserCachePath+`}" -type f \( -name chrome-headless-shell -o -name chrome \) 2>/dev/null); [ -n "$fs" ] || exit 1; for b in $fs; do head -1 "$b" | grep -q '#!/bin/sh' || exit 1; done`).Run() == nil
	check(shimOK, "Playwright browser shimmed to musl chromium", "lerd pest:browser install")

	// The checks above can pass while `lerd test` still fails: pest-plugin-browser
	// boots the server (run-server --mode launchServer) and that boot can fail for
	// a reason presence checks miss. Boot it too and surface the real error (#677).
	if playwrightOK {
		ctx, cancel := context.WithTimeout(context.Background(), pestServerBootTimeout)
		bootOK, bootOut := bootPlaywrightServer(ctx, container, cwd)
		cancel()
		check(bootOK, "Playwright server boots (run-server, launchServer mode)",
			"the server failed to start; its output is below")
		if !bootOK && bootOut != "" {
			fmt.Fprintf(w, "%s\n", indentBlock(bootOut, "        | "))
		}
	}

	return nil
}

// pestServerBootTimeout caps how long the doctor waits for the Playwright server
// to print its ready marker before treating the boot as failed.
const pestServerBootTimeout = 25 * time.Second

// playwrightServerReadyMarker is what Playwright's run-server prints once it is
// listening; pest-plugin-browser waits for the same line before driving it.
const playwrightServerReadyMarker = "Listening on"

// playwrightServerBootCmd mirrors how pest-plugin-browser launches the server:
// run-server in launchServer mode. Port 0 takes an ephemeral port so the probe
// never collides with a real run, and the process is killed as soon as it is up.
const playwrightServerBootCmd = "./node_modules/.bin/playwright run-server --host 127.0.0.1 --port 0 --mode launchServer"

// bootPlaywrightServer boots the Playwright server inside the FPM container the
// way pest-plugin-browser does and reports whether it came up; on failure it
// returns the captured output so the doctor can show the real error.
func bootPlaywrightServer(ctx context.Context, container, cwd string) (bool, string) {
	browsersEnv := "PLAYWRIGHT_BROWSERS_PATH=" + pestBrowserCachePath
	cmd := podman.CmdContext(ctx, "exec", "-w", cwd, "--env", browsersEnv, container,
		"sh", "-c", playwrightServerBootCmd)
	ok, out := watchPlaywrightBoot(ctx, cmd)
	// Killing the podman exec client does not reap the run-server it spawned
	// inside the container. Stop it explicitly; the --port 0 command line is
	// unique to this probe, so a real test run's server is never matched.
	_ = podman.Cmd("exec", container, "pkill", "-f", playwrightServerBootCmd).Run()
	return ok, out
}

// watchPlaywrightBoot starts cmd and waits for the ready marker, returning true
// once listening (then killing it) or false with the captured output if it exits
// first or the context expires. Exec-agnostic so it is testable.
func watchPlaywrightBoot(ctx context.Context, cmd *exec.Cmd) (bool, string) {
	w := &readyWatcher{found: make(chan struct{})}
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		return false, err.Error()
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case <-w.found:
		_ = cmd.Process.Kill()
		<-exited
		return true, ""
	case <-exited:
		out := w.output()
		if ctx.Err() == context.DeadlineExceeded {
			msg := fmt.Sprintf("timed out after %s waiting for the server to start", pestServerBootTimeout)
			if out != "" {
				msg += "\n" + out
			}
			return false, msg
		}
		if out == "" {
			out = "the server exited without output"
		}
		return false, out
	}
}

// readyWatcher accumulates a process's combined output and closes found the
// moment the ready marker appears, so a booting server is detected without
// waiting for it to exit.
type readyWatcher struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	found chan struct{}
	once  sync.Once
}

func (rw *readyWatcher) Write(p []byte) (int, error) {
	rw.mu.Lock()
	rw.buf.Write(p)
	seen := strings.Contains(rw.buf.String(), playwrightServerReadyMarker)
	rw.mu.Unlock()
	if seen {
		rw.once.Do(func() { close(rw.found) })
	}
	return len(p), nil
}

func (rw *readyWatcher) output() string {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return strings.TrimSpace(rw.buf.String())
}

// indentBlock prefixes every line of s with prefix, so multi-line process output
// reads as one indented block under a doctor check.
func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
