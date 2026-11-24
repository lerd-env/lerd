package siteops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// RemoveWorktreeVhosts removes a primary domain's worktree vhosts, except those
// in keep, and returns the domains removed. A registered site's own subdomain
// (a group secondary) matches the suffix scan but is left alone.
func RemoveWorktreeVhosts(primaryDomain string, keep ...string) []string {
	confD := config.NginxConfD()
	entries, err := os.ReadDir(confD)
	if err != nil {
		return nil
	}
	suffix := "." + primaryDomain + ".conf"
	var removed []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		domain := strings.TrimSuffix(e.Name(), ".conf")
		if slices.Contains(keep, domain) {
			continue
		}
		if _, err := config.FindSiteByDomain(domain); err == nil {
			continue
		}
		_ = os.Remove(filepath.Join(confD, e.Name()))
		removed = append(removed, domain)
	}
	return removed
}
