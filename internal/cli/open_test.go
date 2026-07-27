package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// lerd open inside a worktree opens the branch's own domain, not the parent's.
func TestWorktreeURL_usesTheBranchDomainAndParentScheme(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "feature")
	site := &config.Site{Name: "rapids", Path: parent, Domains: []string{"rapids.test"}, Secured: true}

	if got := worktreeURL(site, "feature"); got != "https://feature.rapids.test" {
		t.Errorf("worktreeURL = %q, want https://feature.rapids.test", got)
	}
	site.Secured = false
	if got := worktreeURL(site, "feature"); got != "http://feature.rapids.test" {
		t.Errorf("worktreeURL = %q, want the http form", got)
	}
	if got := worktreeURL(site, "nope"); got != "" {
		t.Errorf("worktreeURL for an unknown branch = %q, want empty", got)
	}
}
