package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// lan:status reported only the global exposure setting, so it said LAN devices
// could not reach anything while a shared site was answering them on its own
// port. A share works whichever way that setting is set, so status has to name
// the ones that are live.
func TestLANStatusNamesActiveShares(t *testing.T) {
	sites := []config.Site{
		{Name: "shop", LANPort: 9100},
		{Name: "idle"},
		{Name: "blog", LANPort: 9101},
	}
	worktrees := []config.WorktreeLANEntry{{Site: "shop", Branch: "feat-x", Port: 9102}}

	lines := lanShareLines(sites, worktrees, "192.168.0.121")
	joined := strings.Join(lines, "\n")

	for _, want := range []string{"shop", "9100", "blog", "9101", "feat-x", "9102", "192.168.0.121"} {
		if !strings.Contains(joined, want) {
			t.Errorf("status should mention %q, got:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "idle") {
		t.Errorf("a site with no share must not be listed:\n%s", joined)
	}
}

func TestLANStatusSilentWithNoShares(t *testing.T) {
	if lines := lanShareLines([]config.Site{{Name: "shop"}}, nil, "192.168.0.121"); len(lines) != 0 {
		t.Errorf("no shares should print nothing, got %v", lines)
	}
}
