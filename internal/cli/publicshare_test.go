package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// assignPublicSharePort hands out the lowest free port from its base, skipping
// ports already taken by other sites and worktrees. Fixture-only: an isolated
// XDG_DATA_HOME so the real registry is never touched.
func TestAssignPublicSharePort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if got := assignPublicSharePort("myapp", ""); got != publicSharePortBase {
		t.Fatalf("empty registry: got %d, want %d", got, publicSharePortBase)
	}

	if err := config.AddSite(config.Site{Name: "a", Domains: []string{"a.test"}, Path: "/a", PublicPort: publicSharePortBase}); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "b", Domains: []string{"b.test"}, Path: "/b",
		WorktreePublicPorts: map[string]int{"feature": publicSharePortBase + 1},
	}); err != nil {
		t.Fatal(err)
	}

	// Base and base+1 are taken (a site and a worktree), so the next is base+2.
	if got := assignPublicSharePort("new", ""); got != publicSharePortBase+2 {
		t.Errorf("got %d, want %d", got, publicSharePortBase+2)
	}
	// Re-assigning site a's own port does not count it as taken.
	if got := assignPublicSharePort("a", ""); got != publicSharePortBase {
		t.Errorf("re-assign a: got %d, want its own %d back", got, publicSharePortBase)
	}
	// Re-assigning b's own worktree does not count it either.
	if got := assignPublicSharePort("b", "feature"); got != publicSharePortBase+1 {
		t.Errorf("re-assign b/feature: got %d, want its own %d back", got, publicSharePortBase+1)
	}
}

func TestSetWorktreePublicPort_setAndClear(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "s", Domains: []string{"s.test"}, Path: "/s"}); err != nil {
		t.Fatal(err)
	}
	if err := setWorktreePublicPort("s", "feat", 9310); err != nil {
		t.Fatal(err)
	}
	site, _ := config.FindSite("s")
	if site.WorktreePublicPorts["feat"] != 9310 {
		t.Errorf("port = %v, want 9310 stored", site.WorktreePublicPorts)
	}
	if err := setWorktreePublicPort("s", "feat", 0); err != nil {
		t.Fatal(err)
	}
	site, _ = config.FindSite("s")
	if _, ok := site.WorktreePublicPorts["feat"]; ok {
		t.Errorf("port should be cleared, got %v", site.WorktreePublicPorts)
	}
}
