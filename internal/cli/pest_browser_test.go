package cli

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// The shim is the whole mechanism: Playwright's glibc browser can't run on
// Alpine musl, so the downloaded binaries must be rewritten to exec the system
// musl chromium with --no-sandbox (Pest never passes it). Guard those invariants.
func TestPestBrowserShim_RewritesToMuslChromium(t *testing.T) {
	for _, want := range []string{
		"/usr/bin/chromium",
		"--no-sandbox",
		"chrome-headless-shell",
		"-name chrome",
		"PLAYWRIGHT_BROWSERS_PATH",
	} {
		if !strings.Contains(pestBrowserShim, want) {
			t.Errorf("shim script missing %q:\n%s", want, pestBrowserShim)
		}
	}
}

// Install must prefer the project's pinned Playwright so the downloaded browser
// revision matches the plugin's expectation, and fail loudly when it is absent.
func TestPestBrowserInstall_PrefersLocalPlaywright(t *testing.T) {
	if !strings.Contains(pestBrowserInstall, "./node_modules/.bin/playwright") {
		t.Error("install script should use the locally installed playwright binary")
	}
	if !strings.Contains(pestBrowserInstall, "lerd npm install playwright") {
		t.Error("install script should hint how to install playwright when missing")
	}
}

// dryRunPlan is real `playwright install --dry-run chromium` output, ffmpeg
// duplication included.
const dryRunPlan = `browser: chromium version 141.0.7390.37
  Install location:    /root/.cache/ms-playwright/chromium-1194
  Download url:        https://cdn.playwright.dev/builds/chromium/1194/chromium-linux-arm64.zip
  Download fallback 1: https://playwright.download.prss.microsoft.com/builds/chromium/1194/chromium-linux-arm64.zip

browser: ffmpeg
  Install location:    /root/.cache/ms-playwright/ffmpeg-1011
  Download url:        https://cdn.playwright.dev/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip
  Download fallback 1: https://playwright.download.prss.microsoft.com/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip

browser: chromium-headless-shell version 141.0.7390.37
  Install location:    /root/.cache/ms-playwright/chromium_headless_shell-1194
  Download url:        https://cdn.playwright.dev/builds/chromium/1194/chromium-headless-shell-linux-arm64.zip
  Download fallback 1: https://playwright.download.prss.microsoft.com/builds/chromium/1194/chromium-headless-shell-linux-arm64.zip

browser: ffmpeg
  Install location:    /root/.cache/ms-playwright/ffmpeg-1011
  Download url:        https://cdn.playwright.dev/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip
  Download fallback 1: https://playwright.download.prss.microsoft.com/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip
`

// The plan parser drives the whole install: one line per component carrying the
// primary URL and the mirror Playwright falls back to when the CDN is
// unreachable, with ffmpeg listed once even though Playwright repeats it per
// browser.
func TestPestBrowserPlanAwk_ParsesDryRun(t *testing.T) {
	cmd := exec.Command("awk", pestBrowserPlanAwk)
	cmd.Stdin = strings.NewReader(dryRunPlan)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("awk: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{
		"/root/.cache/ms-playwright/chromium-1194\thttps://cdn.playwright.dev/builds/chromium/1194/chromium-linux-arm64.zip\thttps://playwright.download.prss.microsoft.com/builds/chromium/1194/chromium-linux-arm64.zip",
		"/root/.cache/ms-playwright/ffmpeg-1011\thttps://cdn.playwright.dev/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip\thttps://playwright.download.prss.microsoft.com/builds/ffmpeg/1011/ffmpeg-linux-arm64.zip",
		"/root/.cache/ms-playwright/chromium_headless_shell-1194\thttps://cdn.playwright.dev/builds/chromium/1194/chromium-headless-shell-linux-arm64.zip\thttps://playwright.download.prss.microsoft.com/builds/chromium/1194/chromium-headless-shell-linux-arm64.zip",
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Playwright only prints fallbacks for components it mirrors, so a block without
// one still has to yield a usable line rather than dropping the component.
func TestPestBrowserPlanAwk_HandlesMissingFallback(t *testing.T) {
	plan := `browser: chromium version 141.0.7390.37
  Install location:    /root/.cache/ms-playwright/chromium-1194
  Download url:        https://cdn.playwright.dev/builds/chromium/1194/chromium-linux-arm64.zip
`
	cmd := exec.Command("awk", pestBrowserPlanAwk)
	cmd.Stdin = strings.NewReader(plan)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("awk: %v", err)
	}
	want := "/root/.cache/ms-playwright/chromium-1194\thttps://cdn.playwright.dev/builds/chromium/1194/chromium-linux-arm64.zip\t"
	if got := strings.TrimRight(string(out), "\n"); got != want {
		t.Errorf("parsed %q, want %q", got, want)
	}
}

// The deadlock in #1006 is Playwright's Node extractor writing into the
// bind-mounted cache, so the install must fetch and unzip the archives itself
// and mark each directory installed the way the registry expects.
func TestPestBrowserInstall_ExtractsWithoutPlaywright(t *testing.T) {
	for _, want := range []string{"--dry-run", "curl -fsSL", "unzip -q -o", "INSTALLATION_COMPLETE"} {
		if !strings.Contains(pestBrowserInstall, want) {
			t.Errorf("install script missing %q:\n%s", want, pestBrowserInstall)
		}
	}
	if !strings.Contains(pestBrowserInstall, "--speed-time") {
		t.Error("a stalled download must abort rather than hang forever")
	}
}

// Playwright retries its own downloads against the Microsoft mirror when the CDN
// is unreachable, which is the normal path on networks that block it. Fetching
// the archives ourselves has to keep that second chance.
func TestPestBrowserInstall_FallsBackToMirror(t *testing.T) {
	for _, want := range []string{`"$url"`, `"$mirror"`} {
		if !strings.Contains(pestBrowserInstall, want) {
			t.Errorf("install script must download from %s:\n%s", want, pestBrowserInstall)
		}
	}
}

// Playwright garbage-collects any browser directory that no .links entry claims,
// so an install that skips the link lets an unrelated project's `playwright
// install` delete these browsers out from under the site that owns them.
func TestPestBrowserInstall_WritesRegistryLink(t *testing.T) {
	for _, want := range []string{".links", "sha1sum", "playwright-core"} {
		if !strings.Contains(pestBrowserInstall, want) {
			t.Errorf("install script missing %q, needed to claim the browsers:\n%s", want, pestBrowserInstall)
		}
	}
}

// The link filename is the sha1 of the playwright-core package path and the
// contents are that same path; Playwright reads it back to resolve the pinned
// revisions, so a different shape silently reads as a broken link.
func TestPestBrowserInstall_LinkNamingMatchesPlaywright(t *testing.T) {
	const pkgPath = "/home/dev/site/node_modules/playwright-core"
	sum := sha1.Sum([]byte(pkgPath))
	want := hex.EncodeToString(sum[:])

	cmd := exec.Command("sh", "-c", `printf '%s' "$1" | sha1sum | cut -d" " -f1`, "sh", pkgPath)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sha1sum: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("link filename = %q, want sha1 of the package path %q", got, want)
	}
}

// An aborted run must not leave the container wedged: the orphaned installer and
// the lock it holds are what make every retry hang instantly with no output.
func TestPestBrowserCleanup_ClearsLockAndOrphans(t *testing.T) {
	for _, want := range []string{"__dirlock", "oopDownloadBrowserMai[n]", "/tmp/playwright-download-*"} {
		if !strings.Contains(pestBrowserCleanup, want) {
			t.Errorf("cleanup script missing %q:\n%s", want, pestBrowserCleanup)
		}
	}
	// The orphan a Ctrl+C leaves behind is now our own curl and the shell driving
	// it, neither of which runs under a Playwright process name.
	if !strings.Contains(pestBrowserCleanup, "lerd-playwrigh[t]") {
		t.Errorf("cleanup must reap lerd's own downloader:\n%s", pestBrowserCleanup)
	}
	// A pattern that matches the cleanup's own command line would kill it before
	// it reaps anything.
	for _, pat := range []string{"oopDownloadBrowserMain", "playwright install", "lerd-playwright"} {
		if strings.Contains(pestBrowserCleanup, pat) {
			t.Errorf("pkill pattern %q matches the cleanup script itself", pat)
		}
	}
}

// chromium is what Playwright drives, and Xvfb is what a headed launch draws on:
// without it Playwright dies telling the user to start an XServer (#1538).
// chromium must stay in the set for another reason too, see the test below.
func TestPestBrowserPkgs_ChromiumAndXvfb(t *testing.T) {
	for _, want := range []string{"chromium", "xvfb", "xvfb-run"} {
		if !slices.Contains(pestBrowserPkgs, want) {
			t.Errorf("pest:browser must bake %q, got %v", want, pestBrowserPkgs)
		}
	}
}

// The image build derives PLAYWRIGHT_BROWSERS_PATH from the chromium package
// alone, so dropping that name would silently unbake the cache path the test
// runner needs.
func TestPestBrowserPkgs_ChromiumDrivesTheBakedEnv(t *testing.T) {
	if !slices.Contains(pestBrowserPkgs, "chromium") {
		t.Fatal("chromium is the marker the FPM build keys PLAYWRIGHT_BROWSERS_PATH off")
	}
}

// A headed launch has to go through xvfb-run, and a headless one has to stay
// direct: Playwright appends --headless itself, so the flag is the only signal
// the shim gets about which mode it was launched for.
func TestPestBrowserShim_HeadedRunsUnderXvfb(t *testing.T) {
	for _, want := range []string{
		`*" --headless"*) exec /usr/bin/chromium --no-sandbox "$@" ;;`,
		`exec xvfb-run -a /usr/bin/chromium --no-sandbox "$@"`,
	} {
		if !strings.Contains(pestBrowserShim, want) {
			t.Errorf("shim missing %q:\n%s", want, pestBrowserShim)
		}
	}
}

// Browser testing needs a modern Node; the frozen legacy 7.4/8.0 tier must be
// rejected up front rather than failing after a multi-minute rebuild.
func TestPestBrowserSupportedVersion(t *testing.T) {
	for _, v := range []string{"7.4", "8.0"} {
		if pestBrowserSupportedVersion(v) == nil {
			t.Errorf("legacy PHP %s must be rejected for browser testing", v)
		}
	}
	for _, v := range []string{"8.3", "8.4", "8.5"} {
		if err := pestBrowserSupportedVersion(v); err != nil {
			t.Errorf("PHP %s should be supported, got %v", v, err)
		}
	}
}

// The shim must shim every browser binary and use a NUL-delimited find so paths
// with spaces or newlines can't corrupt the rewrite. headless_shell is the name
// the chromium_headless_shell build ships, and missing it leaves a glibc binary
// that musl cannot exec while the count guard still passes on chrome alone.
func TestPestBrowserShim_HandlesBothBinariesSafely(t *testing.T) {
	for _, want := range []string{"-name chrome-headless-shell", "-name chrome", "-name headless_shell", "-print0", "read -r -d ''"} {
		if !strings.Contains(pestBrowserShim, want) {
			t.Errorf("shim missing %q:\n%s", want, pestBrowserShim)
		}
	}
}

// Running the generated wrapper is the only way to prove the mode split works:
// the heredoc, the case glob and the flag Playwright appends all have to line up,
// and a string match would pass on a wrapper that picks the wrong branch.
func TestPestBrowserShim_WrapperPicksModeAtRuntime(t *testing.T) {
	cache := t.TempDir()
	bin := filepath.Join(cache, "chrome-linux")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	browser := filepath.Join(bin, "chrome")
	if err := os.WriteFile(browser, []byte("glibc binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// bash, not sh: the rewrite loop reads NUL-delimited paths with `read -d`,
	// which the container's busybox ash supports and a host /bin/sh may not.
	// Under a shell that lacks it the loop silently rewrites nothing while the
	// count guard still passes, so the wrapper check below is what catches it.
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available to run the generator")
	}
	gen := exec.Command(bash, "-c", pestBrowserShim)
	gen.Env = append(os.Environ(), "PLAYWRIGHT_BROWSERS_PATH="+cache)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generating the shim failed: %v\n%s", err, out)
	}

	// The wrapper hardcodes the container's chromium path and looks xvfb-run up
	// on PATH; point both at stubs that just announce which one ran.
	wrapper, err := os.ReadFile(browser)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(wrapper), "#!/bin/sh") {
		t.Fatalf("the browser binary was not rewritten to a wrapper:\n%s", wrapper)
	}
	stub := filepath.Join(cache, "fake-chromium")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'chromium'; printf ' [%s]' \"$@\"; echo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "xvfb-run"), []byte("#!/bin/sh\necho \"xvfb-run $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(browser, []byte(strings.ReplaceAll(string(wrapper), "/usr/bin/chromium", stub)), 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		cmd := exec.Command(browser, args...)
		cmd.Env = append(os.Environ(), "PATH="+cache+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("wrapper %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	if got := run("--headless", "--remote-debugging-pipe"); strings.Contains(got, "xvfb-run") {
		t.Errorf("headless launch went through xvfb-run: %s", got)
	}
	if got := run("--remote-debugging-pipe"); !strings.HasPrefix(got, "xvfb-run -a ") {
		t.Errorf("headed launch did not get a virtual display: %s", got)
	}

	// The container's DNS cannot answer wildcard subdomains for chromium (#2374),
	// so each linked site's subdomains go to Pest's loopback server while the
	// names /etc/hosts already points at nginx keep resolving there.
	hosts := filepath.Join(cache, "hosts")
	hostsBody := "127.0.0.1 localhost\n::1 localhost\n192.168.127.254 host.containers.internal host.docker.internal\n10.89.0.81 myapp.test\n10.89.0.81 admin.myapp.test\n"
	if err := os.WriteFile(hosts, []byte(hostsBody), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapper, err = os.ReadFile(browser)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(browser, []byte(strings.ReplaceAll(string(wrapper), "/etc/hosts", hosts)), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "[--host-resolver-rules=EXCLUDE admin.myapp.test, EXCLUDE myapp.test, MAP *.admin.myapp.test 127.0.0.1, MAP *.myapp.test 127.0.0.1] [--headless]"
	if got := run("--headless"); !strings.Contains(got, want) {
		t.Errorf("resolver rules not built from the hosts file:\n got %s\nwant %s", got, want)
	}

	if err := os.WriteFile(hosts, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run("--headless"); strings.Contains(got, "host-resolver-rules") {
		t.Errorf("resolver rules passed with no linked sites: %s", got)
	}
}

// The extractor marks the binaries executable by name, so it has to know the
// same set the shim rewrites or headless_shell lands without the +x bit.
func TestPestBrowserExtract_ChmodsHeadlessShell(t *testing.T) {
	if !strings.Contains(pestBrowserInstall, "-name 'headless_shell'") {
		t.Errorf("extractor does not chmod headless_shell:\n%s", pestBrowserInstall)
	}
}

// The cli cache path must stay equal to the podman source of truth that bakes
// the image ENV and the volume mount target.
func TestPestBrowserCachePathMatchesPodman(t *testing.T) {
	if pestBrowserCachePath != podman.PlaywrightCachePath {
		t.Errorf("cache path drift: cli=%q podman=%q", pestBrowserCachePath, podman.PlaywrightCachePath)
	}
}

func TestNewPestBrowserCmd_HasSubcommands(t *testing.T) {
	cmd := NewPestBrowserCmd()
	if cmd.Use != "pest:browser" {
		t.Errorf("parent command Use = %q, want pest:browser", cmd.Use)
	}
	want := map[string]bool{"install": false, "remove": false, "doctor": false}
	for _, sub := range cmd.Commands() {
		name := strings.Fields(sub.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("pest:browser missing %q subcommand", name)
		}
	}
}

// The boot probe must invoke run-server in launchServer mode, exactly the way
// pest-plugin-browser does, or it would test a different code path than the one
// that fails for users (#677).
func TestPlaywrightServerBootCmd_LaunchServerMode(t *testing.T) {
	for _, want := range []string{"./node_modules/.bin/playwright", "run-server", "--mode launchServer"} {
		if !strings.Contains(playwrightServerBootCmd, want) {
			t.Errorf("boot command missing %q: %s", want, playwrightServerBootCmd)
		}
	}
}

// Boot succeeds the moment the ready marker is printed, even though the server
// then keeps running; the watcher must detect it and stop the process.
func TestWatchPlaywrightBoot_DetectsReadyMarker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'Listening on ws://127.0.0.1:1/'; sleep 30")
	ok, out := watchPlaywrightBoot(ctx, cmd)
	if !ok {
		t.Fatalf("expected boot success, got false with output %q", out)
	}
}

// The whole point of #677: when the server dies before listening, the doctor
// must surface the process's real output, not a bare failure.
func TestWatchPlaywrightBoot_SurfacesOutputOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'boom: the real reason' >&2; exit 1")
	ok, out := watchPlaywrightBoot(ctx, cmd)
	if ok {
		t.Fatal("expected boot failure")
	}
	if !strings.Contains(out, "boom: the real reason") {
		t.Errorf("expected the real output surfaced, got %q", out)
	}
}

func TestWatchPlaywrightBoot_TimesOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 30")
	ok, out := watchPlaywrightBoot(ctx, cmd)
	if ok {
		t.Fatal("expected boot failure on timeout")
	}
	if !strings.Contains(out, "timed out") {
		t.Errorf("expected a timeout message, got %q", out)
	}
}

// A deadline that passes before the server even starts is still a timeout, as
// it is on a loaded CI runner where the start itself is slow.
func TestWatchPlaywrightBoot_TimesOutBeforeStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	ok, out := watchPlaywrightBoot(ctx, exec.CommandContext(ctx, "sh", "-c", "sleep 30"))
	if ok || !strings.Contains(out, "timed out") {
		t.Errorf("watchPlaywrightBoot = %v, %q, want a timeout message", ok, out)
	}
}

func TestIndentBlock_PrefixesEveryLine(t *testing.T) {
	got := indentBlock("first\nsecond", "  | ")
	want := "  | first\n  | second"
	if got != want {
		t.Errorf("indentBlock = %q, want %q", got, want)
	}
}

func TestPestBrowserAllowedInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := nativePestBrowserRefusal(); err != nil {
		t.Errorf("container mode must allow pest:browser, got: %v", err)
	}
}
