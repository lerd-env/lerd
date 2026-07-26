package node

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// SystemNodeBinDirs resolves the directories where an unmanaged node and npm
// actually live, so the host-worker generators can bake them into a unit's
// PATH. A generated unit never inherits the user's login PATH, so a Node
// injected by a shell hook (nvm, a self-installed fnm, volta, mise, asdf) or
// living outside the standard dirs (snap, linuxbrew) is invisible at runtime
// even though `node` works fine in the user's terminal — issue #1143.
//
// Resolution order mirrors detectSystemNode at install time: the current PATH
// first (skipping lerd's own shim dir), then the well-known version-manager
// install layouts, then static locations. Returns nil when nothing usable is
// found.
func SystemNodeBinDirs() []string {
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

// pathNodeBinDirs walks PATH for the first dirs holding node and npm, skipping
// lerd's own bin dir so a stale managed-node shim never counts as a system
// install. node and npm can resolve to different dirs (e.g. a distro node with
// a separately installed npm); both are returned, node's dir first. complete
// reports whether npm was found too.
func pathNodeBinDirs() (dirs []string, complete bool) {
	lerdBin := config.BinDir()
	var nodeDir, npmDir string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
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

// nvmNodeBinDir resolves an nvm install's bin dir. The default alias wins when
// it directly names an installed version (matching what `nvm use default`
// would run); alias indirections like lts/* fall through to the newest one.
func nvmNodeBinDir() string {
	versions := filepath.Join(nvmDir(), "versions", "node")
	if alias, err := os.ReadFile(filepath.Join(nvmDir(), "alias", "default")); err == nil {
		v := strings.TrimSpace(string(alias))
		for _, name := range []string{v, "v" + v} {
			if dir := filepath.Join(versions, name, "bin"); name != "" && hasNodeAndNpm(dir) {
				return dir
			}
		}
	}
	return bestVersionBin(versions, "bin")
}

// bestVersionBin scans base for version-named subdirectories and returns the
// newest one's bin dir (joined via sub) that holds both node and npm, or "".
func bestVersionBin(base string, sub ...string) string {
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
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
