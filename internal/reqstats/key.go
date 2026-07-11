package reqstats

import "strings"

// Key is the identity every request-timing view is stored and queried under: the
// site name, or "<site>/<branch>" for a git worktree. The branch is the sanitized
// one the HTTP API, MCP and the worktree registries already share, so a reader
// asking for a branch and the watcher writing one land on the same row. Worker
// units and idle state key a worktree by its checkout directory instead; the two
// schemes are deliberately separate, and this is the only one the store knows.
func Key(site, branch string) string {
	if branch == "" {
		return site
	}
	return site + "/" + branch
}

// SplitKey parses a key back into its site and branch. branch is empty for a
// main-site key, which a site name can never look like since it can't contain "/".
func SplitKey(key string) (site, branch string) {
	if i := strings.IndexByte(key, '/'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}
