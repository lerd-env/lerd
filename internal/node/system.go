package node

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
)

// SystemNodeBinDirs resolves the directories where an unmanaged node and npm
// actually live, so the host-worker generators can bake them into a unit's
// PATH. A generated unit never inherits the user's login PATH, so a Node
// injected by a shell hook (nvm, a self-installed fnm, volta, mise, asdf) or
// living outside the standard dirs (snap, linuxbrew) is invisible at runtime
// even though `node` works fine in the user's terminal — issue #1143.
//
// A configured manager lerd does not own comes first: `lerd npm` runs through
// it, so resolving some other node from PATH would install modules under one
// Node and run them under another. Otherwise the order mirrors detectSystemNode
// at install time: the current PATH first (skipping lerd's own shim dir), then
// the well-known version-manager install layouts, then static locations.
// Returns nil when nothing usable is found.
func SystemNodeBinDirs() []string {
	return SystemNodeBinDirsFor("")
}

// SystemNodeBinDirsFor is SystemNodeBinDirs for a site pinned to version (a
// major like "22", empty for no pin). Only a borrowed manager can honour the
// pin: it is the version `lerd npm` runs in that directory, so a worker on any
// other one would run modules that were installed under a different Node. An
// uninstalled pin falls back rather than installing into the user's manager.
func SystemNodeBinDirsFor(version string) []string {
	if dir := borrowedManagerBinDir(version); dir != "" {
		return []string{dir}
	}
	pathDirs, complete := pathNodeBinDirs()
	if complete {
		return pathDirs
	}
	// PATH has no npm (bare distro node, or nothing): a version-manager or
	// static install with the full toolchain beats a node-only dir.
	if dir := managerNodeBinDir(); dir != "" {
		return []string{dir}
	}
	for _, dir := range []string{"/snap/bin", "/home/linuxbrew/.linuxbrew/bin"} {
		if hasNodeAndNpm(dir) {
			return []string{dir}
		}
	}
	// Last resort: a node without npm still serves `node server.js` workers.
	return pathDirs
}

// unitPathDirs are the prefixes the generated worker unit puts on PATH itself
// while a daemon's own PATH omits them: on macOS launchd hands lerd-ui and
// lerd-watcher /usr/bin:/bin:/usr/sbin:/sbin, so a Homebrew node is invisible to
// them and a unit written by the dashboard resolved a different Node than the
// same unit written by the CLI. Walking them with PATH keeps the answer equal on
// both routes, and equal to what the unit runs. A var so tests can drive it.
var unitPathDirs = func() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return hostbin.ExtraDirs()
}

// pathNodeBinDirs walks PATH for the first dirs holding node and npm, skipping
// lerd's own bin dir so a stale managed-node shim never counts as a system
// install. node and npm can resolve to different dirs (e.g. a distro node with
// a separately installed npm); both are returned, node's dir first. complete
// reports whether npm was found too.
func pathNodeBinDirs() (dirs []string, complete bool) {
	lerdBin := config.BinDir()
	var nodeDir, npmDir string
	for _, dir := range append(filepath.SplitList(os.Getenv("PATH")), unitPathDirs()...) {
		if dir == "" || dir == lerdBin {
			continue
		}
		if nodeDir == "" && isExecutable(filepath.Join(dir, "node")) {
			nodeDir = dir
		}
		if npmDir == "" && isExecutable(filepath.Join(dir, "npm")) {
			npmDir = dir
		}
		if nodeDir != "" && npmDir != "" {
			break
		}
	}
	if nodeDir == "" {
		return nil, false
	}
	if npmDir != "" && npmDir != nodeDir {
		return []string{nodeDir, npmDir}, true
	}
	return []string{nodeDir}, npmDir == nodeDir
}

// borrowedManagerBinDir returns the node bin dir of a configured manager that
// belongs to the user rather than to lerd, which today means nvm: lerd only
// ever borrows an nvm install, while fnm is its own bundled tool and must not
// override the user's Node on a host where they declined managed Node. version
// selects a specific major when the manager has it, otherwise its default
// applies. Empty when the manager is fnm or has nothing installed, so the
// caller falls through to the usual resolution.
func borrowedManagerBinDir(version string) string {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil || cfg.NodeManager() != "nvm" {
		return ""
	}
	if version != "" {
		if dir := nvmAliasBinDir(filepath.Join(nvmDir(), "versions", "node"), version); dir != "" {
			return dir
		}
	}
	return nvmNodeBinDir()
}

// managerNodeBinDir probes the version-manager install layouts a shell hook
// would activate, in the same order detectSystemNode reports them. Only a dir
// containing both node and npm qualifies.
func managerNodeBinDir() string {
	if dir := nvmNodeBinDir(); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	if dir := filepath.Join(home, ".volta", "bin"); hasNodeAndNpm(dir) {
		return dir
	}
	if dir := bestVersionBin(filepath.Join(home, ".local", "share", "mise", "installs", "node"), "bin"); dir != "" {
		return dir
	}
	if dir := bestVersionBin(filepath.Join(home, ".asdf", "installs", "nodejs"), "bin"); dir != "" {
		return dir
	}
	for _, fnmBase := range []string{
		filepath.Join(home, ".local", "share", "fnm"),
		filepath.Join(home, "Library", "Application Support", "fnm"),
	} {
		if dir := filepath.Join(fnmBase, "aliases", "default", "bin"); hasNodeAndNpm(dir) {
			return dir
		}
		if dir := bestVersionBin(filepath.Join(fnmBase, "node-versions"), "installation", "bin"); dir != "" {
			return dir
		}
	}
	return ""
}

// nvmNodeBinDir resolves an nvm install's bin dir, matching what `nvm use
// default` would run: the default alias when it names an installed version or
// pins a major like "20", since that is how most people set a default. Alias
// indirections lerd cannot resolve without nvm itself (lts/*, another alias)
// fall through to the newest install.
func nvmNodeBinDir() string {
	versions := filepath.Join(nvmDir(), "versions", "node")
	if alias, err := os.ReadFile(filepath.Join(nvmDir(), "alias", "default")); err == nil {
		if dir := nvmAliasBinDir(versions, strings.TrimSpace(string(alias))); dir != "" {
			return dir
		}
	}
	return bestVersionBin(versions, "bin")
}

// nvmAliasBinDir resolves the default alias value v against the installed
// versions: an exact directory match first, then the newest install under a
// partial pin ("20", "20.10"). Returns "" for anything non-numeric.
func nvmAliasBinDir(versions, v string) string {
	if v == "" {
		return ""
	}
	for _, name := range []string{v, "v" + v} {
		if dir := filepath.Join(versions, name, "bin"); hasNodeAndNpm(dir) {
			return dir
		}
	}
	bare := strings.TrimPrefix(v, "v")
	if bare == "" || strings.Trim(bare, "0123456789.") != "" {
		return ""
	}
	return bestVersionBinUnder(versions, "v"+bare, "bin")
}

// bestVersionBin scans base for version-named subdirectories and returns the
// newest one's bin dir (joined via sub) that holds both node and npm, or "".
func bestVersionBin(base string, sub ...string) string {
	return bestVersionBinUnder(base, "", sub...)
}

// bestVersionBinUnder is bestVersionBin restricted to versions under a pin, so
// prefix "v20" matches v20.20.2 but not v200.1.0. An empty prefix accepts all.
func bestVersionBinUnder(base, prefix string, sub ...string) string {
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if prefix != "" && e.Name() != prefix && !strings.HasPrefix(e.Name(), prefix+".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool { return versionLess(names[j], names[i]) })
	for _, name := range names {
		dir := filepath.Join(append([]string{base, name}, sub...)...)
		if hasNodeAndNpm(dir) {
			return dir
		}
	}
	return ""
}

// versionLess compares two version-directory names ("v22.9.1", "20.10.0")
// numerically, segment by segment. Non-numeric segments compare as 0 so odd
// names sort last rather than breaking the scan.
func versionLess(a, b string) bool {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var an, bn int
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		if an != bn {
			return an < bn
		}
	}
	return false
}

// hasNodeAndNpm reports whether dir holds executable node and npm entries.
func hasNodeAndNpm(dir string) bool {
	return isExecutable(filepath.Join(dir, "node")) && isExecutable(filepath.Join(dir, "npm"))
}

// isExecutable reports whether path is an executable file (following
// symlinks, since npm is normally a symlink into lib/node_modules).
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

// CommandUsesNode reports whether any segment of a worker's shell command
// invokes the Node toolchain, so a missing Node only holds back workers that
// would actually crash on it (a Go or Python host-proxy command in a repo
// that happens to carry a package.json must keep running directly).
func CommandUsesNode(command string) bool {
	uses := false
	mapSegments(command, func(seg string) string {
		switch tok, _, _ := segmentVerb(seg); tok {
		case "node", "npm", "npx", "yarn", "pnpm", "corepack":
			uses = true
		}
		return seg
	})
	return uses
}
