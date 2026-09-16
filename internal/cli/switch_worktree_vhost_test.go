package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// A worktree is not a registry entry of its own, so the runtime switch's loop
// over sites never reached one. Its vhost kept pointing at the upstream the
// other runtime served, and every worktree answered 502 while its parent was
// fine.
func TestRuntimeSwitchRegeneratesWorktreeVhosts(t *testing.T) {
	prev := switchWorktreesFor
	t.Cleanup(func() { switchWorktreesFor = prev })
	switchWorktreesFor = func(string, string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{
			{Branch: "feat-x", Domain: "feat-x.demo.test", Path: "/p/demo-feat-x"},
			{Branch: "feat-z", Domain: "feat-z.demo.test", Path: "/p/demo-feat-z"},
		}, nil
	}

	var written []string
	prevWrite := switchWriteWorktreeVhost
	t.Cleanup(func() { switchWriteWorktreeVhost = prevWrite })
	switchWriteWorktreeVhost = func(domain, path, _, _, _, _ string, _ bool) error {
		written = append(written, domain+" -> "+path)
		return nil
	}

	site := &config.Site{Name: "demo", Path: "/p/demo", PHPVersion: "8.5", Domains: []string{"demo.test"}}
	regenerateWorktreeVhosts(site)

	want := []string{"feat-x.demo.test -> /p/demo-feat-x", "feat-z.demo.test -> /p/demo-feat-z"}
	if len(written) != len(want) {
		t.Fatalf("wrote %v, want %v", written, want)
	}
	for i := range want {
		if written[i] != want[i] {
			t.Errorf("wrote %q, want %q", written[i], want[i])
		}
	}
}
