package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewOpenCmd returns the open command.
func NewOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [site]",
		Short: "Open the current site in the default browser",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runOpen,
	}
}

// worktreeURL is the address a branch is served on, under the parent's scheme.
// Empty when the branch has no worktree.
func worktreeURL(site *config.Site, branch string) string {
	domain, err := siteops.WorktreeDomain(site, branch)
	if err != nil {
		return ""
	}
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	return scheme + "://" + domain
}

func runOpen(_ *cobra.Command, args []string) error {
	var url string

	if len(args) > 0 {
		// Name provided — look it up directly.
		site, err := config.FindSite(args[0])
		if err != nil {
			return fmt.Errorf("site %q not found", args[0])
		}
		url = siteURL(site.Path)
	} else {
		// No argument — use cwd.
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		url = siteURL(cwd)
		if url == "" {
			// A worktree is served on its own subdomain, so open that rather
			// than the parent site it inherits its registration from.
			if parent, branch, ok := findOwningWorktree(cwd); ok {
				url = worktreeURL(parent, branch)
			}
		}
		if url == "" {
			// Fall back: maybe cwd is named like a site.
			name, _ := siteops.SiteNameAndDomain(filepath.Base(cwd), "test")
			if site, err := config.FindSite(name); err == nil {
				url = siteURL(site.Path)
			}
		}
		if url == "" {
			site, err := ensureSiteForCwd()
			if err != nil {
				return err
			}
			url = siteURL(site.Path)
		}
	}

	if url == "" {
		return errNotLinked()
	}

	fmt.Printf("Opening %s\n", url)
	return openBrowser(url)
}
