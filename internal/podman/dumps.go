package podman

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

//go:embed dumpbridge
var dumpBridgeFS embed.FS

// DumpBridgePHP returns the embedded contents of dump-bridge.php as a string.
// Exposed for tests so they can assert the on-disk file is byte-identical to
// the embed without re-reading the embed FS.
func DumpBridgePHP() (string, error) {
	b, err := dumpBridgeFS.ReadFile("dumpbridge/dump-bridge.php")
	if err != nil {
		return "", fmt.Errorf("debug bridge embed: %w", err)
	}
	return string(b), nil
}

// DevtoolsCollectorPHP returns the embedded devtools-collector.php. The dump
// bridge's auto_prepend requires it for its shared transport, so it rides the
// same down-to-7.2 parse constraint and tests assert that.
func DevtoolsCollectorPHP() (string, error) {
	b, err := dumpBridgeFS.ReadFile("dumpbridge/devtools-collector.php")
	if err != nil {
		return "", fmt.Errorf("devtools collector embed: %w", err)
	}
	return string(b), nil
}

// DumpBridgeIni returns the conf.d ini content with runtime placeholders
// substituted. {{ DUMP_TARGET }} resolves through config.DumpsBridgeTarget
// — `unix://…` on Linux (reachable via the standard %h:%h bind mount) or
// `tcp://host.containers.internal:9913` on macOS where unix sockets don't
// traverse podman-machine's virtio-fs boundary. {{ DUMP_PASSTHROUGH }} is
// "1" or "0" depending on Dumps.Passthrough.
func DumpBridgeIni() (string, error) {
	b, err := dumpBridgeFS.ReadFile("dumpbridge/97-lerd-dump.ini")
	if err != nil {
		return "", fmt.Errorf("debug bridge ini embed: %w", err)
	}
	target := config.DumpsBridgeTarget()
	passthrough := "0"
	if cfg, _ := config.LoadGlobal(); cfg != nil && cfg.IsDumpsPassthrough() {
		passthrough = "1"
	}
	out := strings.ReplaceAll(string(b), "{{ DUMP_TARGET }}", target)
	out = strings.ReplaceAll(out, "{{ DUMP_PASSTHROUGH }}", passthrough)
	return out, nil
}

// DevtoolsSeamsConf renders every job seam the framework store declares, one
// per line as kind|match|target|method|name.
//
// The union of all frameworks is written rather than only the linked ones: a
// seam names classes that exist solely in the app that has them, so a line for
// a framework this machine doesn't run can never fire, and the file needs no
// regeneration when a site is linked.
func DevtoolsSeamsConf() string {
	var lines []string
	seen := map[string]bool{}
	for _, fw := range config.ListFrameworks() {
		if fw == nil || fw.Devtools == nil {
			continue
		}
		for _, seam := range fw.Devtools.Jobs {
			match, target := seam.Target()
			name := seam.Name
			if name == "" {
				name = "this"
			}
			if target == "" || seam.Method == "" {
				continue
			}
			line := strings.Join([]string{"job", match, target, seam.Method, name}, "|")
			// A field carrying the separator or a newline would shift every
			// field after it, so drop the seam rather than write a broken line.
			if strings.ContainsAny(target+seam.Method+name, "|\n\r") || seen[line] {
				continue
			}
			seen[line] = true
			lines = append(lines, line)
		}
	}
	sort.Strings(lines)
	out := "# lerd devtools capture seams, generated from the framework store.\n" +
		"# kind|match|target|method|name, read once per PHP process.\n"
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

// WriteDumpBridgeAssets writes the bridge PHP file and the conf.d ini to
// their host paths under DataDir()/php/dumps/. Idempotent: a regular file
// whose contents already match the embed is left untouched. Replaces a
// directory at the same path that podman might have auto-created earlier.
func WriteDumpBridgeAssets() error {
	dir := config.DumpsAssetsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating dumps dir: %w", err)
	}

	phpContent, err := DumpBridgePHP()
	if err != nil {
		return err
	}
	iniContent, err := DumpBridgeIni()
	if err != nil {
		return err
	}
	adapterContent, err := dumpBridgeFS.ReadFile("dumpbridge/laravel-adapter.php")
	if err != nil {
		return fmt.Errorf("laravel adapter embed: %w", err)
	}
	collectorContent, err := dumpBridgeFS.ReadFile("dumpbridge/devtools-collector.php")
	if err != nil {
		return fmt.Errorf("devtools collector embed: %w", err)
	}

	for _, asset := range []struct {
		path    string
		content string
	}{
		{config.DumpsBridgeFile(), phpContent},
		{config.DumpsIniFile(), iniContent},
		{config.LaravelAdapterFile(), string(adapterContent)},
		{config.DevtoolsCollectorFile(), string(collectorContent)},
		{config.DevtoolsSeamsFile(), DevtoolsSeamsConf()},
	} {
		if info, err := os.Stat(asset.path); err == nil {
			if info.IsDir() {
				if rmErr := os.RemoveAll(asset.path); rmErr != nil {
					return fmt.Errorf("removing stale dump asset directory %s: %w", asset.path, rmErr)
				}
			} else if existing, readErr := os.ReadFile(asset.path); readErr == nil && string(existing) == asset.content {
				continue
			}
		}
		if err := os.WriteFile(asset.path, []byte(asset.content), 0644); err != nil {
			return fmt.Errorf("writing dump asset %s: %w", asset.path, err)
		}
	}
	return nil
}

// RemoveDumpAssets deletes the host-side bridge file, ini, and enable flag.
// Used by `lerd uninstall` and tests; not called on `lerd dump off` because
// the assets are always-mounted into FPM and removing them would force a
// container restart on the next FPM start. Safe to call repeatedly.
func RemoveDumpAssets() error {
	for _, p := range []string{
		config.DumpsBridgeFile(),
		config.DumpsIniFile(),
		config.LaravelAdapterFile(),
		config.DevtoolsCollectorFile(),
		config.DevtoolsSeamsFile(),
		config.DumpsEnabledFlagFile(),
		config.DevtoolsWorkersFlagFile(),
		// Legacy: the devtools collector used to have its own enable sentinel
		// before it was unified onto enabled.flag. Clean up any stale copy so
		// an old install doesn't leave an orphan nothing reads.
		filepath.Join(config.DumpsAssetsDir(), "devtools.flag"),
	} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", p, err)
		}
	}
	dir := config.DumpsAssetsDir()
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
	return nil
}

// EnsureDumpAssets guarantees the bridge PHP file and conf.d ini exist as
// regular files on disk so podman doesn't auto-create directories at the
// bind-mount source paths when an FPM container first starts. Always runs
// regardless of Dumps.Enabled because the FPM quadlet always mounts these
// paths; the bridge's runtime sentinel check controls active behaviour.
func EnsureDumpAssets() error {
	for _, p := range []string{config.DumpsBridgeFile(), config.DumpsIniFile(), config.LaravelAdapterFile(), config.DevtoolsCollectorFile(), config.DevtoolsSeamsFile()} {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if rmErr := os.RemoveAll(p); rmErr != nil {
				return fmt.Errorf("removing stale dump asset directory %s: %w", p, rmErr)
			}
		}
	}
	return WriteDumpBridgeAssets()
}

// SetDumpsBridgeFlag flips the on-disk sentinel the bridge reads on every
// request. Touching the flag = capture is on; removing it = capture is off.
// No container action is needed because the file path is always
// volume-mounted into every FPM container.
func SetDumpsBridgeFlag(enabled bool) error {
	flag := config.DumpsEnabledFlagFile()
	if enabled {
		if err := os.MkdirAll(filepath.Dir(flag), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(flag, []byte("1\n"), 0644); err != nil {
			return fmt.Errorf("writing dumps flag: %w", err)
		}
		return nil
	}
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing dumps flag: %w", err)
	}
	return nil
}
