package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
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

// A failed bind must not leave the chosen port in the registry. Persisting it
// first meant every later start retried the same dead port and the site could
// never be shared publicly again.
func TestPublicShareStartLeavesNoPortBehindWhenTheBindFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, err := SetPublicBaseDomain("dev.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}

	// Occupy the port the assigner will hand out, so the bind fails.
	blocker, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", publicSharePortBase))
	if err != nil {
		t.Skipf("cannot occupy port %d: %v", publicSharePortBase, err)
	}
	defer blocker.Close()

	if _, err := PublicShareStart("myapp"); err == nil {
		t.Fatal("PublicShareStart succeeded despite an occupied port")
	}

	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if site.PublicPort != 0 {
		t.Errorf("PublicPort = %d after a failed bind, want 0", site.PublicPort)
	}
}

// A site may only be shared one way at a time. The daemon enforced it on start
// but the CLI's port-reservation path did not, so a LAN port could be persisted
// onto a publicly shared site and both listeners bound on the next restore.
func TestLANShareEnsurePortRefusesAPubliclySharedSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := config.AddSite(config.Site{
		Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}

	publicShareMu.Lock()
	publicShareServers["myapp"] = &http.Server{}
	publicShareMu.Unlock()
	defer func() {
		publicShareMu.Lock()
		delete(publicShareServers, "myapp")
		publicShareMu.Unlock()
	}()

	if _, err := LANShareEnsurePort("myapp"); !errors.Is(err, errShareBusy) {
		t.Errorf("LANShareEnsurePort error = %v, want errShareBusy", err)
	}
	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if site.LANPort != 0 {
		t.Errorf("LANPort = %d, want 0: no port should be reserved for a busy site", site.LANPort)
	}
}
