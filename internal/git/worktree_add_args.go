package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
func DeriveWorktreeAddArgs(sitePath string, args []string) []string {
	if hasWorktreePathArg(args) {
		return args
	}

	// `worktree add -b <branch> <path> [<start-point>]`
	for i, a := range args {
		if (a == "-b" || a == "-B") && i+1 < len(args) {
			path := WorktreeCheckoutPath(sitePath, args[i+1])
			out := append([]string{}, args[:i+2]...)
			out = append(out, path)
			return append(out, args[i+2:]...)
		}
	}

	// `worktree add <path> <branch>` for an existing branch, which is the shape
	// the dashboard builds too. Only with exactly one positional: a second one
	// is a commit-ish or a path we have no business reordering.
	flags, positionals := splitWorktreeArgs(args)
	if len(positionals) != 1 {
		return args
	}
	branch := positionals[0]
	out := append([]string{}, flags...)
	return append(out, WorktreeCheckoutPath(sitePath, branch), branch)
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

// WorktreeCheckoutPath returns the directory a new worktree for branch should
// be checked out into: a child of the parent site, "<sitePath>/<base>-<slug>"
// where <base> is filepath.Base(sitePath). Bumps a numeric suffix if the
// default path already exists. RunWorktreeAdd also writes a `/<base>-*/`
// pattern into .git/info/exclude so git status doesn't show siblings.
// Caveat: non-git tools (composer, IDEs, find/rsync/tar) walking the
// parent tree DO descend into the worktree — gitignore doesn't hide it
// from them. Most callers don't care; flag if you do.
func WorktreeCheckoutPath(sitePath, branch string) string {
	parentBase := filepath.Base(sitePath)
	base := filepath.Join(sitePath, parentBase+"-"+SanitizeBranch(branch))
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
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
