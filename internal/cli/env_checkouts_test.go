package cli

import (
	"strings"
	"testing"

	gitpkg "github.com/geodro/lerd/internal/git"
)

// A worktree keeps its own env and is not a site in the registry, so a sweep
// that walks the registry rewrites the main checkout and leaves every worktree
// pointing at whatever it was wired to when it was created. The vhost sweep
// beside it already covers them.
func TestEnvCheckoutPaths(t *testing.T) {
	cases := []struct {
		name      string
		sitePath  string
		worktrees []gitpkg.Worktree
		want      string
	}{
		{"no worktrees", "/srv/app", nil, "/srv/app"},
		{
			"each worktree follows the main checkout",
			"/srv/app",
			[]gitpkg.Worktree{{Path: "/srv/app-feature"}, {Path: "/srv/app-hotfix"}},
			"/srv/app,/srv/app-feature,/srv/app-hotfix",
		},
		// A worktree whose checkout is gone would send lerd env at nothing.
		{
			"a worktree with no path is skipped",
			"/srv/app",
			[]gitpkg.Worktree{{Path: ""}, {Path: "/srv/app-feature"}},
			"/srv/app,/srv/app-feature",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(envCheckoutPaths(c.sitePath, c.worktrees), ",")
			if got != c.want {
				t.Errorf("envCheckoutPaths = %q, want %q", got, c.want)
			}
		})
	}
}
