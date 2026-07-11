package reqstats

import "testing"

// The store key round-trips, and a worktree key stays distinct from a site whose
// name happens to look like one. Writer and readers share this function precisely
// so a worktree's rows can't be written under one spelling and queried under
// another, which is what left the request-timing panel empty on every worktree.
func TestKeyRoundTrip(t *testing.T) {
	cases := []struct {
		site, branch, want string
	}{
		{"myapp", "", "myapp"},
		{"myapp", "feature-auth", "myapp/feature-auth"},
		{"my-app", "fix-123", "my-app/fix-123"},
	}
	for _, c := range cases {
		got := Key(c.site, c.branch)
		if got != c.want {
			t.Errorf("Key(%q, %q) = %q, want %q", c.site, c.branch, got, c.want)
		}
		site, branch := SplitKey(got)
		if site != c.site || branch != c.branch {
			t.Errorf("SplitKey(%q) = (%q, %q), want (%q, %q)", got, site, branch, c.site, c.branch)
		}
	}
}
