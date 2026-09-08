// Package nativephp runs PHP-FPM on the host instead of in a container, for
// sites whose runtime is "native". macOS bind-mounts the project into the
// podman VM, so a containerised PHP pays a virtiofs crossing on every file it
// reads; running on the host removes that boundary entirely.
package nativephp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// portBase is the first port of the range native FPM listeners occupy. Chosen
// above the 9000 fastcgi convention so a native listener never collides with
// the containerised FPM a site may be switched back to.
const portBase = 9400

// PortFor returns the loopback port the native FPM for a PHP version listens
// on. Derived from the version rather than allocated, so it survives restarts
// with nothing persisted and stays the same across machines.
func PortFor(version string) (int, error) {
	major, minor, ok := strings.Cut(version, ".")
	if !ok {
		return 0, fmt.Errorf("php version %q: want <major>.<minor>", version)
	}
	maj, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("php version %q: bad major: %w", version, err)
	}
	min, err := strconv.Atoi(minor)
	if err != nil {
		return 0, fmt.Errorf("php version %q: bad minor: %w", version, err)
	}
	if maj < 5 || maj > 9 || min < 0 || min > 9 {
		return 0, fmt.Errorf("php version %q out of range", version)
	}
	return portBase + maj*10 + min, nil
}

// IniScanDirs returns the directories a native FPM scans for php.ini fragments,
// in load order. These are the same files the containers bind-mount into
// conf.d, so php:ini, dumps, the debug window and xdebug behave identically
// whichever runtime a site is on. Order matches the containers' numeric conf.d
// prefixes (94-mail, 95-shared, 96-devtools, 97-dump, then the per-version
// 98-user and 99-xdebug).
func IniScanDirs(version string) []string {
	return []string{
		filepath.Dir(config.MailIniFile()),
		filepath.Dir(config.SharedIniFile()),
		filepath.Dir(config.DevtoolsIniFile()),
		filepath.Dir(config.DumpsIniFile()),
		// SPX is configured by an ini of its own. A directory the pool does not
		// scan is one it never reads, which left spx.http_enabled unset and the
		// profiler dashboard blank.
		filepath.Dir(config.SpxIniFile()),
		filepath.Dir(config.PHPUserIniFile(version)),
		OverrideDir(version),
	}
}

// OverrideDir holds the ini fragment that rewrites container paths to host
// ones. Scanned last so it wins over the copies the containers share.
func OverrideDir(version string) string {
	return filepath.Join(config.RunDir(), "native", version, "conf.d")
}

// overrideIni re-points the settings whose values name container paths. The
// debug bridge is auto-prepended from a directory that exists only in the
// image, and PHP fatals on an auto_prepend_file it cannot open, so both the
// prepend and the bridge's own asset lookup are moved to their host copies.
// The capture socket is reached over loopback rather than the container's host
// gateway, since here PHP is already on the host.
func overrideIni(version string) string {
	return overrideIniWith(DevtoolsExtensionPath(version))
}

// DevtoolsExtensionPath is where a native build's query-capture extension lives
// when the build ships one. It is the engine-level collector behind the Debug
// window's query lens, compiled into the PHP image and shipped beside the
// binary here, the same way xdebug is.
//
// Named per version because a PHP module is built against one PHP's ABI: an
// 8.4 module will not load into 8.5, and offering it would warn on every
// request while the lens stayed empty.
func DevtoolsExtensionPath(version string) string {
	path := filepath.Join(ModulesDir(version), "lerd_devtools-"+version+".so")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// ModulesDir holds the shared objects a native build ships beside its binary.
// Per version, because they are named for the extension and not for the PHP
// they were built against: one directory would leave every version loading
// whichever xdebug.so was installed last, and a module only loads into the ABI
// it was compiled for.
func ModulesDir(version string) string {
	return filepath.Join(config.DataDir(), "native-php", version, "modules")
}

func overrideIniWith(devtoolsSO string) string {
	assets := config.DumpsAssetsDir()
	return "; lerd: native runtime overrides. Auto-generated, do not edit.\n" +
		"auto_prepend_file=" + config.DumpsBridgeFile() + "\n" +
		"lerd.assets_dir=" + assets + "\n" +
		"lerd.dump_host=tcp://127.0.0.1:9913\n" +
		"lerd.devtools_host=tcp://127.0.0.1:9913\n" +
		"lerd.devtools_flag=" + filepath.Join(assets, "enabled.flag") + "\n" +
		// SPX writes its profiles to a path that exists only inside the image.
		"spx.data_dir=" + config.SpxDataDir() + "\n" +
		devtoolsLine(devtoolsSO)
}

// devtoolsLine loads the query-capture extension when the build shipped one.
// Naming a file that is not there would make PHP warn on every single request.
// It registers a zend_module_entry, so it loads with extension= despite using
// the zend_observer API internally; zend_extension= is for the other kind and
// PHP refuses the module outright.
func devtoolsLine(path string) string {
	if path == "" {
		return ""
	}
	return "extension=" + path + "\n"
}

// WriteOverrides materialises the override fragment for a version.
func WriteOverrides(version string) error {
	dir := OverrideDir(version)
	config.GuardRealWrite(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "zz-native.ini"), []byte(overrideIni(version)), 0644)
}

// IniScanDir joins IniScanDirs into the PHP_INI_SCAN_DIR value PHP expects.
func IniScanDir(version string) string {
	return strings.Join(IniScanDirs(version), string(filepath.ListSeparator))
}

// FPMConfig renders the php-fpm.conf for a PHP version's native listener.
// Pool sizing mirrors the containerised pool so switching a site's runtime
// doesn't silently change how much concurrency it can absorb. PHP settings are
// deliberately absent: they come from IniScanDir, the same files the containers
// mount, so php:ini stays the single place they are edited.
func FPMConfig(version, errorLog string) (string, error) {
	port, err := PortFor(version)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; Generated by lerd for PHP %s. Edits are overwritten.\n", version)
	fmt.Fprintf(&b, "; PHP settings live in php:ini, not here.\n\n")
	b.WriteString("[global]\n")
	fmt.Fprintf(&b, "error_log = %s\n", errorLog)
	b.WriteString("daemonize = no\n\n")
	b.WriteString("[www]\n")
	// nginx connects from inside the podman VM, so a loopback-only bind would
	// refuse it. The pool is still reachable only from this machine: the port is
	// not published anywhere and the VM is local.
	fmt.Fprintf(&b, "listen = %d\n", port)
	// ondemand rather than dynamic: a laptop leaves sites untouched for hours,
	// and dynamic holds start_servers children resident per version the whole
	// time. The master owns the opcache shared memory, so a pool that has
	// fallen to zero costs a fork on the next request and nothing more.
	b.WriteString("pm = ondemand\n")
	b.WriteString("pm.max_children = 20\n")
	b.WriteString("pm.process_idle_timeout = 60s\n")
	b.WriteString("clear_env = no\n")
	return b.String(), nil
}

// BinaryPath is where lerd keeps the native PHP-FPM for a version. Suffixed
// with the minor because a project's composer.lock was resolved against one,
// and deliberately not "php": that name in BinDir is the shim into the
// container, and a static binary written over it would send every php on the
// machine to a PHP with no lerd behind it.
func BinaryPath(version string) string {
	return filepath.Join(config.BinDir(), "php-native-"+version)
}

// EnsureSupported rejects a version no native build will ever exist for. A
// permanent limit is not a missing download, and the two need different words
// or the reader waits for a build that is never coming.
func EnsureSupported(version string) error {
	if !Supported(version) {
		return fmt.Errorf("php %s has no native runtime (needs %s or newer); move the site up or keep this install on the container runtime", version, MinVersion)
	}
	return nil
}

// EnsureInstalled reports whether the native runtime for a version is usable,
// naming what is missing rather than letting nginx fastcgi into a dead port.
func EnsureInstalled(version, binary string) error {
	if _, err := PortFor(version); err != nil {
		return err
	}
	if err := EnsureSupported(version); err != nil {
		return err
	}
	info, err := os.Stat(binary)
	if err != nil {
		return fmt.Errorf("PHP %s is not installed (%s)", version, binary)
	}
	if info.IsDir() || info.Mode()&0111 == 0 {
		return fmt.Errorf("PHP %s at %s is not an executable", version, binary)
	}
	return nil
}

// FPMBinaryPath is where lerd keeps the native PHP-FPM for a version. Separate
// from BinaryPath because FPM and CLI are different SAPIs and different builds:
// FPM serves requests, the CLI runs artisan, composer and tinker.
func FPMBinaryPath(version string) string {
	return filepath.Join(config.BinDir(), "php-native-fpm-"+version)
}

// ShimDir holds a "php" symlink to the native CLI for a version. Workers run
// their command through a shell, so this goes on PATH ahead of BinDir, whose
// own "php" is the shim into the container. Without it a native site's workers
// would keep running containerised while its web requests did not.
func ShimDir(version string) string {
	return filepath.Join(config.RunDir(), "native", version, "bin")
}

// EnsureShim points ShimDir's php at the native CLI, replacing a stale link so
// a version switch or reinstall is picked up.
func EnsureShim(version, cliBinary string) (string, error) {
	dir := ShimDir(version)
	config.GuardRealWrite(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "php")
	if current, err := os.Readlink(link); err == nil && current == cliBinary {
		return dir, nil
	}
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Symlink(cliBinary, link); err != nil {
		return "", err
	}
	return dir, nil
}

// Extensions reports what a version's native build carries, read from the
// binary itself so it cannot drift from what is actually installed. The list
// is the module names PHP prints, which callers should canonicalise before
// comparing (PHP prints OPcache as "Zend OPcache").
func Extensions(version string) ([]string, error) {
	binary := BinaryPath(version)
	if err := EnsureInstalled(version, binary); err != nil {
		return nil, err
	}
	// Scanned the same way the pool scans, so what is reported is what the
	// version actually loads: the compiled-in set plus the loadable ones lerd
	// enables beside it (xdebug, the query collector). Without this it answered
	// with the bare binary, which is not what anything runs.
	cmd := exec.Command(binary, "-m")
	cmd.Env = append(os.Environ(), "PHP_INI_SCAN_DIR="+IniScanDir(version))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading native php %s extensions: %w", version, err)
	}
	var mods []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		mods = append(mods, line)
	}
	return mods, nil
}

// MinVersion is the oldest PHP with a native runtime. Below it the toolchain
// cannot produce a usable binary: static PHP carries no OPcache before 8.0, and
// 7.4 and 8.0 both fail to compile against the libxml2 and ICU the build uses,
// which would cost dom, simplexml, soap, xsl and intl even if they linked.
const MinVersion = "8.1"

// Supported reports whether a PHP version has a native runtime.
func Supported(version string) bool {
	major, minor, ok := strings.Cut(version, ".")
	if !ok {
		return false
	}
	maj, err := strconv.Atoi(major)
	if err != nil {
		return false
	}
	min, err := strconv.Atoi(minor)
	if err != nil {
		return false
	}
	return maj > 8 || (maj == 8 && min >= 1)
}

// ListInstalled returns the PHP versions with a usable native runtime on disk,
// sorted. A version needs both SAPIs: the CLI runs artisan, composer and the
// workers, and FPM serves requests, so a half-installed version would list as
// available and then fail at whichever half is missing.
func ListInstalled() []string {
	entries, err := os.ReadDir(config.BinDir())
	if err != nil {
		return nil
	}
	cli, fpm := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		if v, ok := strings.CutPrefix(e.Name(), "php-native-fpm-"); ok {
			fpm[v] = true
			continue
		}
		if v, ok := strings.CutPrefix(e.Name(), "php-native-"); ok {
			cli[v] = true
		}
	}
	var out []string
	for v := range cli {
		if fpm[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// ToolName is the manifest entry and version-stamp name for a version's host
// build. Shared so the CLI that installs it and the dashboard that reports on
// it cannot disagree about what the stamp beside the binary is called.
func ToolName(version string) string { return "php-native-" + version }
