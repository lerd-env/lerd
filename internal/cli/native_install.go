package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// nativeTool names the manifest entry for a PHP minor. Deliberately the same
// name as the CLI binary in BinDir, so the version stamp the tools package
// keeps beside a tool lands next to the file it describes.
func nativeTool(version string) string { return "php-native-" + version }

// nativeFetchTimeout bounds the whole download. The archive carries two static
// binaries and the shared extensions, so it is tens of megabytes and worth a
// longer ceiling than a single tool.
const nativeFetchTimeout = 15 * time.Minute

// unpackNativePHP moves an extracted build into place and records the patch it
// is. The modules directory is replaced wholesale rather than merged: the
// extensions are named for the extension and not for the build, so a leftover
// .so from the previous patch would keep its name and fail to load into the
// binary that just replaced it.
func unpackNativePHP(stage, version, patch string) error {
	mods := nativephp.ModulesDir(version)
	if err := os.MkdirAll(filepath.Dir(mods), 0o755); err != nil {
		return err
	}
	// Renamed aside rather than deleted first, so a failure here leaves the
	// previous build serving instead of no modules at all.
	old := mods + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(mods); err == nil {
		if err := os.Rename(mods, old); err != nil {
			return fmt.Errorf("replacing the modules for php %s: %w", version, err)
		}
	}
	if err := os.Rename(filepath.Join(stage, "modules"), mods); err != nil {
		os.Rename(old, mods) //nolint:errcheck
		return fmt.Errorf("installing the modules for php %s: %w", version, err)
	}
	os.RemoveAll(old)

	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"php-native-" + version, "php-native-fpm-" + version} {
		src := filepath.Join(stage, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("the php %s build is missing %s", version, name)
		}
		dest := filepath.Join(config.BinDir(), name)
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("installing %s: %w", name, err)
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return err
		}
	}
	// Stamped last: the stamp is only true once every file it describes is in
	// place, and it is ignored anyway when it predates the binary beside it.
	return tools.WriteStamp(nativeTool(version), patch)
}

// installNativePHP downloads the pinned patch for a PHP minor and swaps it in,
// returning the patch installed. Callers restart FPM afterwards: a running
// pool holds its binary and its extensions open and keeps serving the build it
// started with until it is told otherwise.
func installNativePHP(pins *pinnedTools, version string, w io.Writer) (string, error) {
	tool := nativeTool(version)
	if pins.m == nil {
		pins.m = tools.Load(context.Background())
	}
	pin, ok := pins.m.Tools[tool]
	if !ok {
		return "", fmt.Errorf("lerd publishes no native php %s build for %s/%s",
			version, runtime.GOOS, runtime.GOARCH)
	}

	stage, err := os.MkdirTemp(config.DataDir(), "native-php-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)

	archive := filepath.Join(stage, "php.tar.gz")
	ctx, cancel := context.WithTimeout(context.Background(), nativeFetchTimeout)
	defer cancel()
	step := feedback.StartOn(w, "fetching php "+pin.Version)
	// The downloader draws a progress bar, which is a stream of frames anywhere
	// that is not a terminal; the step spinner stays one line either way.
	if _, err := pins.downloadCtx(ctx, tool, archive, 0o644, io.Discard); err != nil {
		step.Fail(err)
		return "", fmt.Errorf("downloading php %s: %w", pin.Version, err)
	}
	step.OK("downloaded")

	extract := exec.Command("tar", "-xzf", archive, "-C", stage)
	if out, err := extract.CombinedOutput(); err != nil {
		return "", fmt.Errorf("extracting php %s: %v: %s", pin.Version, err, out)
	}
	if err := unpackNativePHP(stage, version, pin.Version); err != nil {
		return "", err
	}
	return pin.Version, nil
}

// nativeUpdatePlan decides what a version needs against the published pin.
// Any difference is worth acting on, not just a higher number: the pin is
// lerd's statement of what it publishes, and a machine sitting on a build that
// has been withdrawn should come back to it.
func nativeUpdatePlan(pinned, installed string) (string, bool) {
	if pinned == "" {
		return "", false
	}
	return pinned, pinned != installed
}

// ensureNativePHPInstalled downloads a version's native build when it is not
// already on disk. Switching runtimes used to refuse outright on a missing
// binary, which left the person to work out what to fetch and from where; the
// build is published, so lerd fetches it.
func ensureNativePHPInstalled(pins *pinnedTools, version string, w io.Writer) error {
	if err := nativephp.EnsureSupported(version); err != nil {
		return err
	}
	_, cliErr := os.Stat(nativephp.BinaryPath(version))
	_, fpmErr := os.Stat(nativephp.FPMBinaryPath(version))
	if cliErr == nil && fpmErr == nil {
		return nil
	}
	feedback.Line("php " + version + " has no native build installed yet")
	_, err := installNativePHP(pins, version, w)
	return err
}
