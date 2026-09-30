package config

import (
	"path/filepath"
	"slices"
	"strings"
)

// HomeSpellings returns every path that names the home directory: home itself,
// its resolved form, and /home/<user> when that leads to it. On ostree hosts
// (Silverblue, Bazzite) /home links to /var/home, so a shell, git and $HOME can
// each spell the same folder a different way.
func HomeSpellings(home string) []string {
	return HomeSpellingsUnder(home, "/home")
}

// HomeSpellingsUnder is HomeSpellings with the /home root passed in.
func HomeSpellingsUnder(home, homeRoot string) []string {
	if home == "" {
		return nil
	}
	out := []string{home}
	for _, p := range []string{CanonicalPath(home), filepath.Join(homeRoot, filepath.Base(home))} {
		if !slices.Contains(out, p) && SamePath(p, home) {
			out = append(out, p)
		}
	}
	return out
}

// PathWithin reports whether p is root or lies under it, whichever spelling of
// a symlinked folder either one arrives in. A p that no longer exists is
// resolved through the part of it that still does, so a removed worktree still
// counts as inside its project.
func PathWithin(p, root string) bool {
	if p == "" || root == "" {
		return false
	}
	return within(filepath.Clean(p), filepath.Clean(root)) ||
		within(resolveExisting(p), resolveExisting(root))
}

func within(p, root string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}

// resolveExisting resolves symlinks in the longest leading part of p that
// exists and keeps the rest as written.
func resolveExisting(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
