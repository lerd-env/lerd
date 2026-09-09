package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/download"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/imagepull"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// pinnedTools resolves host-tool download URLs for the current platform,
// loading the manifest lazily so an install with nothing missing never
// fetches it. Returns the pinned version so callers can stamp it.
type pinnedTools struct{ m *tools.Manifest }

func (p *pinnedTools) download(name, dest string, mode os.FileMode, w io.Writer) (string, error) {
	return p.downloadCtx(context.Background(), name, dest, mode, w)
}

// downloadCtx is download under a caller's context, so a fetch that would
// otherwise sit inside the retry loop for as long as the server keeps trickling
// bytes has an end.
func (p *pinnedTools) downloadCtx(ctx context.Context, name, dest string, mode os.FileMode, w io.Writer) (string, error) {
	if p.m == nil {
		p.m = tools.Load(ctx)
	}
	url, err := p.m.URL(name, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	digest := p.m.Digest(name, runtime.GOOS, runtime.GOARCH)
	if err := download.Verified(ctx, url, dest, mode, digest, w); err != nil {
		return "", err
	}
	return p.m.Tools[name].Version, nil
}

// ensureFnmBinary installs fnm into BinDir when it is missing. Called from
// downloadBinaries on a normal (fnm) install, and on demand when switching
// back to fnm with `lerd node:manager fnm` after an nvm-only setup.
func ensureFnmBinary(w io.Writer) error {
	if _, err := os.Stat(filepath.Join(config.BinDir(), "fnm")); err == nil {
		return nil
	}
	var pins pinnedTools
	return installFnm(&pins, w)
}

// installFnm downloads and extracts the pinned fnm, overwriting any existing
// binary, and stamps the installed version.
func installFnm(pins *pinnedTools, w io.Writer) error {
	binDir := config.BinDir()
	fnmZip := filepath.Join(binDir, "fnm.zip")
	v, err := pins.download("fnm", fnmZip, 0644, w)
	if err != nil {
		return fmt.Errorf("fnm download: %w", err)
	}
	extractCmd := exec.Command("unzip", "-o", fnmZip, "fnm", "-d", binDir)
	extractCmd.Stdout = w
	extractCmd.Stderr = w
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("fnm extract: %w", err)
	}
	os.Remove(fnmZip)
	os.Chmod(filepath.Join(binDir, "fnm"), 0755) //nolint:errcheck
	_ = tools.WriteStamp("fnm", v)
	return nil
}

// replaceTool downloads the pinned build of a single-binary tool next to its
// destination and swaps it in atomically, then stamps the version.
func replaceTool(pins *pinnedTools, name, dest string, w io.Writer) error {
	v, err := pins.download(name, dest+".new", 0755, w)
	if err != nil {
		os.Remove(dest + ".new")
		return err
	}
	if err := os.Rename(dest+".new", dest); err != nil {
		return err
	}
	_ = tools.WriteStamp(name, v)
	return nil
}

// hostPHPPath is where lerd keeps the full PHP it runs a host command with.
// Deliberately not "php": that name in BinDir is the shim into the container,
// and writing a static binary over it would send every `php` on the machine to
// a PHP with no lerd behind it. Suffixed with the minor it is, since a project
// runs its own code and its composer.lock was resolved against one version.
func hostPHPPath(tool string) string { return filepath.Join(config.BinDir(), tool) }

// hostPHPTool picks the pinned build for a project's PHP version: the matching
// minor, or the nearest pinned one when a project runs a PHP lerd pins no build
// for. Nearest rather than newest, because a project on 8.1 is closer to 8.3
// than to whatever the newest line has deprecated since.
func hostPHPTool(m *tools.Manifest, phpVersion string) (string, bool) {
	want := "php-host-" + phpVersion
	if _, ok := m.Tools[want]; ok {
		return want, true
	}
	best, bestGap := "", 0
	for name := range m.Tools {
		minor, ok := strings.CutPrefix(name, "php-host-")
		if !ok {
			continue
		}
		gap := versionGap(phpVersion, minor)
		if best == "" || gap < bestGap || (gap == bestGap && minor < strings.TrimPrefix(best, "php-host-")) {
			best, bestGap = name, gap
		}
	}
	return best, best != ""
}

// versionGap measures how far two "8.3"-shaped versions are apart in minors, so
// the nearest pinned build can be picked without ordering rules spread around.
func versionGap(a, b string) int {
	n := func(v string) int {
		major, minor, _ := strings.Cut(v, ".")
		maj, _ := strconv.Atoi(major)
		min, _ := strconv.Atoi(minor)
		return maj*100 + min
	}
	gap := n(a) - n(b)
	if gap < 0 {
		return -gap
	}
	return gap
}

// ensureHostPHPBinary installs the pinned static PHP for a project's PHP version
// when it is missing or behind the pin, and answers where it is. Downloaded on
// demand rather than at install time, because only a project whose bundled
// runtime is short an extension ever needs it.
// installedNativeHostPHP returns the native runtime's binary for a version when
// one is installed. The native build is a full static PHP for the same version,
// so a host command that needs an extension the bundled binary lacks can use it
// directly instead of downloading a second, slimmer build alongside it.
func installedNativeHostPHP(phpVersion string, path func(string) string) (string, bool) {
	p := path(phpVersion)
	info, err := os.Stat(p)
	if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
		return "", false
	}
	return p, true
}

func ensureHostPHPBinary(w io.Writer, phpVersion string) (string, error) {
	// Prefer the native runtime's binary when it is already installed: it is
	// the same static PHP for the same version, with a wider extension set than
	// the pinned fallback, so downloading that too would duplicate it.
	if p, ok := installedNativeHostPHP(phpVersion, nativephp.BinaryPath); ok {
		return p, nil
	}
	pins := &pinnedTools{m: tools.Load(context.Background())}
	tool, ok := hostPHPTool(pins.m, phpVersion)
	if !ok {
		return "", fmt.Errorf("lerd pins no PHP build to fall back to")
	}
	dest := hostPHPPath(tool)
	if _, err := os.Stat(dest); err == nil && tools.InstalledVersion(tool) == pins.m.Tools[tool].Version {
		return dest, nil
	}
	version := pins.m.Tools[tool].Version
	on := feedback.ColorFor(w)
	fmt.Fprintf(w, "%s%s %s\n", feedback.Prefix, feedback.DimIf(on, feedback.GlyphDownload),
		hostPHPDownloadLabel(version, pins.m.Size(tool, runtime.GOOS, runtime.GOARCH)))
	// Extracted through a directory of its own: the archive's member is called
	// "php", and unpacking that straight into BinDir would land on the shim.
	stage, err := os.MkdirTemp(config.BinDir(), "php-host-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)

	archive := filepath.Join(stage, "php.tar.gz")
	ctx, cancel := context.WithTimeout(context.Background(), hostPHPFetchTimeout)
	defer cancel()
	// The downloader draws its own bar, which is a stream of frames anywhere
	// that is not a terminal. The step spinner is the one lerd uses everywhere
	// else and stays a single line either way, so the bar goes to the bin.
	step := feedback.StartOn(w, "fetching php "+version)
	v, err := pins.downloadCtx(ctx, tool, archive, 0644, io.Discard)
	if err != nil {
		step.Fail(err)
		return hostPHPFallback(dest, tools.InstalledVersion(tool), w, err)
	}
	step.OK("downloaded")
	extract := exec.Command("tar", "-xzf", archive, "-C", stage, "php")
	extract.Stdout = w
	extract.Stderr = w
	if err := extract.Run(); err != nil {
		return "", fmt.Errorf("php extract: %w", err)
	}
	if err := os.Rename(filepath.Join(stage, "php"), dest); err != nil {
		return "", err
	}
	os.Chmod(dest, 0755) //nolint:errcheck
	_ = tools.WriteStamp(tool, v)
	return dest, nil
}

// hostPHPFetchTimeout bounds the whole fetch. download.Verified already retries
// a stalled attempt three times against a 60s idle watchdog, which on a server
// that keeps trickling bytes is a wait with no end; a command a user is
// watching needs one.
const hostPHPFetchTimeout = 10 * time.Minute

// hostPHPFallback answers a failed fetch with the copy already on disk when
// there is one. A pin that moved is not worth failing a command over: the build
// installed last time has the extensions the command needs just as much, and it
// says so rather than running a version the user was not told about.
func hostPHPFallback(dest, installed string, w io.Writer, cause error) (string, error) {
	if _, err := os.Stat(dest); err != nil {
		return "", fmt.Errorf("could not download the PHP this command needs: %w", cause)
	}
	if installed == "" {
		installed = "the copy already installed"
	}
	feedback.WarnOn(w, "could not download the pinned PHP (%v), running with %s instead", cause, installed)
	return dest, nil
}

// hostPHPDownloadLabel says what is about to be fetched and how big it is, so a
// command that pauses to download tens of megabytes discloses that first, the
// way an image pull does.
func hostPHPDownloadLabel(version string, size int64) string {
	label := "lerd downloads PHP " + version + " for this command"
	if size > 0 {
		label += " (" + imagepull.Human(size) + ")"
	}
	return label
}
