package cli

import (
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

func TestMatchWorktreeKey(t *testing.T) {
	wts := []gitpkg.Worktree{
		{Name: "wt", Branch: "feature-x", Path: "/home/u/Code/app-feature-x", Domain: "feature-x.app.test"},
		{Name: "wt", Branch: "bugfix/login", Path: "/home/u/Code/app-bugfix-login", Domain: "bugfix-login.app.test"},
	}

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"main checkout", "/home/u/Code/app", ""},
		{"worktree root", "/home/u/Code/app-feature-x", "app/" + config.WorktreeUnitSlug("app-feature-x")},
		{"nested in worktree", "/home/u/Code/app-feature-x/database/migrations", "app/" + config.WorktreeUnitSlug("app-feature-x")},
		{"second worktree", "/home/u/Code/app-bugfix-login/app", "app/" + config.WorktreeUnitSlug("app-bugfix-login")},
		{"sibling prefix not a match", "/home/u/Code/app-feature-x-extra", ""},
		{"trailing slash normalised", "/home/u/Code/app-feature-x/", "app/" + config.WorktreeUnitSlug("app-feature-x")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchWorktreeKey("app", filepath.Clean(tc.dir), wts); got != tc.want {
				t.Errorf("matchWorktreeKey(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}
