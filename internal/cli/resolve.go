package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/config"
)

// errNotLinked is the single message every directory-scoped command shows when
// the current directory has no registered site, replacing several earlier
// phrasings so the guidance is consistent everywhere.
func errNotLinked() error {
	return fmt.Errorf("no site registered for this directory — run 'lerd link' first")
}

// ensureSiteForCwd resolves the site for the current working directory. A
// worktree resolves to its parent; callers that act on the branch itself want
// ensureSiteAndBranchForCwd.
func ensureSiteForCwd() (*config.Site, error) {
	site, _, err := ensureSiteAndBranchForCwd()
	return site, err
}

// ensureSiteAndBranchForCwd resolves the site for the current working
// directory, using os.Getwd for both lookup and link so they can't diverge. On
// a miss in an interactive terminal it offers to link (cascading into init) and
// re-resolves. The branch is empty unless the directory is a worktree.
func ensureSiteAndBranchForCwd() (*config.Site, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	if site, err := config.FindSiteByPath(cwd); err == nil {
		return site, "", nil
	}
	// A worktree inherits the parent's registration, so an exact lookup always
	// misses. Resolve it ahead of the link prompt, which only leads to link
	// refusing it and the caller advising a command that cannot work.
	if parent, branch, ok := findOwningWorktree(cwd); ok {
		return parent, branch, nil
	}
	if !isInteractive() {
		return nil, "", errNotLinked()
	}

	// Wrap the prompt and link in envInterrupt so, when reached from `lerd env`
	// under its live spinner, the prompt and wizard print cleanly instead of
	// being clobbered by the spinner's redraw. Outside runEnvLive it just runs.
	var linkErr error
	envInterrupt(func() {
		fmt.Print("This directory isn't linked to lerd. Link it now? [Y/n] ")
		var answer string
		fmt.Scanln(&answer) //nolint:errcheck
		if !(answer == "" || answer[0] == 'Y' || answer[0] == 'y') {
			linkErr = errNotLinked()
			return
		}
		// runLinkOrInit, not runLink, so a fresh non-PHP/empty project gets the
		// same init wizard `lerd link` now offers instead of a bare PHP link.
		linkErr = runLinkOrInit(nil)
	})
	if linkErr != nil {
		return nil, "", linkErr
	}
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		return nil, "", errNotLinked()
	}
	return site, "", nil
}
