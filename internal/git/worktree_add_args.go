package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// DeriveWorktreeAddArgs supplies the checkout path `git worktree add` requires
// when the user did not type one. The docs describe every form without a path
// (`lerd worktree add -b feat-x`), and the dashboard honours that by computing
// the path through WorktreeCheckoutPath, but the CLI forwarded its arguments to
// git untouched, so each documented form died on git's usage message.
//
// Only the two unambiguous shapes are filled in: an explicit new branch, and a
// single bare existing branch. Anything else is passed through exactly as typed,
// because guessing which positional is a path and which is a commit-ish is how
// a wrapper starts checking out the wrong thing.
func DeriveWorktreeAddArgs(sitePath string, args []string) ([]string, error) {
	if hasWorktreePathArg(args) {
		return args, nil
	}

	// `worktree add -b <branch> <path> [<start-point>]`
	for i, a := range args {
		if (a == "-b" || a == "-B") && i+1 < len(args) {
			// A positional before -b is git's own path slot, marker or not.
			if _, before := splitWorktreeArgs(args[:i]); len(before) > 0 {
				return args, nil
			}
			path, err := WorktreeCheckoutPath(sitePath, args[i+1])
			if err != nil {
				return nil, err
			}
			out := append([]string{}, args[:i+2]...)
			out = append(out, path)
			return append(out, args[i+2:]...), nil
		}
	}

	// `worktree add <path> <branch>` for an existing branch, which is the shape
	// the dashboard builds too. Only with exactly one positional: a second one
	// is a commit-ish or a path we have no business reordering.
	flags, positionals := splitWorktreeArgs(args)
	if len(positionals) != 1 {
		return args, nil
	}
	branch := positionals[0]
	path, err := WorktreeCheckoutPath(sitePath, branch)
	if err != nil {
		return nil, err
	}
	out := append([]string{}, flags...)
	return append(out, path, branch), nil
}

// hasWorktreePathArg reports whether the user already named a directory. Only a
// leading path marker counts: a branch may contain slashes (`feature/auth`), so
// a separator alone says nothing.
func hasWorktreePathArg(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if strings.HasPrefix(a, "/") || strings.HasPrefix(a, "./") ||
			strings.HasPrefix(a, "../") || strings.HasPrefix(a, "~") {
			return true
		}
	}
	return false
}

// splitWorktreeArgs separates flags from positionals. A flag's own value is not
// distinguished, so this is only trusted where the -b forms have already been
// handled above.
func splitWorktreeArgs(args []string) (flags, positionals []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			continue
		}
		positionals = append(positionals, a)
	}
	return flags, positionals
}

// WorktreeCheckoutPath returns "<root>/<site>-<branch>", root being the site or,
// with worktree_location: sibling, its parent, so IDEs don't index every branch
// with the site. A numeric suffix is bumped while the path exists.
func WorktreeCheckoutPath(sitePath, branch string) (string, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	root := sitePath
	switch cfg.WorktreeLocation {
	case "", "inside":
	case "sibling":
		root = filepath.Dir(sitePath)
		if !containersSee(cfg, root) {
			return "", fmt.Errorf("worktree_location: sibling puts this worktree in %s, which lerd's containers do not mount; add it under mounts: in config.yaml", root)
		}
	default:
		return "", fmt.Errorf("worktree_location %q in config.yaml is not one of inside, sibling", cfg.WorktreeLocation)
	}
	base := filepath.Join(root, filepath.Base(sitePath)+"-"+SanitizeBranch(branch))
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

// EnsureNestedWorktreeExclude appends "/<base>-*/" to .git/info/exclude in
// sitePath (idempotent) so nested worktree dirs don't show up in git status
// of the parent repo. No-op when .git isn't a dir or when basename
// contains gitignore meta-chars we'd have to escape.
func EnsureNestedWorktreeExclude(sitePath string) error {
	gitInfo, err := os.Stat(filepath.Join(sitePath, ".git"))
	if err != nil || !gitInfo.IsDir() {
		return nil
	}
	base := filepath.Base(sitePath)
	if strings.ContainsAny(base, "[]?*!#\\") {
		return nil
	}
	return AppendExclude(sitePath, "/"+base+"-*/")
}

// ExcludeLocalOverride keeps .lerd.local.yaml, which lerd writes, out of git
// status. A project that isn't a git checkout is left alone.
func ExcludeLocalOverride(sitePath string) error {
	info, err := os.Stat(filepath.Join(sitePath, ".git"))
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return AppendExclude(sitePath, "/"+config.LocalOverrideFile)
	}
	// A worktree's .git is a file; its excludes live in the shared git dir.
	common, err := Output(sitePath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	return AppendExclude(filepath.Dir(strings.TrimSpace(common)), "/"+config.LocalOverrideFile)
}

// AppendExclude adds pattern to the site's .git/info/exclude once.
func AppendExclude(sitePath, pattern string) error {
	excludePath := filepath.Join(sitePath, ".git", "info", "exclude")
	existing, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}
	_, err = f.WriteString(prefix + pattern + "\n")
	return err
}

// containersSee reports whether the containers already mount dir: home, or a
// parked directory or mounts: entry over it. lerd never mounts it on its own,
// since the folder beside a site can be /usr/local, over the image's own files.
func containersSee(cfg *config.GlobalConfig, dir string) bool {
	if home, _ := os.UserHomeDir(); home != "" && config.PathWithin(dir, home) {
		return true
	}
	for _, m := range append(append([]string{}, cfg.ParkedDirectories...), cfg.Mounts...) {
		if config.PathWithin(dir, m) {
			return true
		}
	}
	return false
}
