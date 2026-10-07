package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/shims"
)

func TestServiceDetail_ShowsPortsLine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := serviceops.SetExtraPorts("mysql", []string{"8080:80"}); err != nil {
		t.Fatal(err)
	}
	if _, err := serviceops.SetPublishedPort("mysql", 33907); err != nil {
		t.Fatal(err)
	}
	m := NewModel("test")
	svc := &ServiceRow{Name: "mysql", State: stateRunning}
	joined := stripANSI(strings.Join(m.serviceHeader(svc, 140), "\n"))
	if !strings.Contains(joined, "localhost:33907") {
		t.Errorf("expected ports line with the moved port:\n%s", joined)
	}
	if !strings.Contains(joined, "default 3306") {
		t.Errorf("expected the default-port hint:\n%s", joined)
	}
	if !strings.Contains(joined, "8080:80") {
		t.Errorf("expected the extra port mapping:\n%s", joined)
	}
}

func TestServiceDetail_RendersHeader(t *testing.T) {
	m := NewModel("test")
	svc := &ServiceRow{Name: "redis", Version: "7.2.4", State: stateRunning}
	joined := stripANSI(strings.Join(m.serviceHeader(svc, 120), "\n"))
	if !strings.Contains(joined, "redis") {
		t.Errorf("expected service name in header:\n%s", joined)
	}
	if !strings.Contains(joined, "7.2.4") {
		t.Errorf("expected version:\n%s", joined)
	}
	if !strings.Contains(joined, "running") {
		t.Errorf("expected state line:\n%s", joined)
	}
}

func TestServiceDetail_ListsDependencies(t *testing.T) {
	m := NewModel("test")
	svc := &ServiceRow{Name: "phpmyadmin", State: stateRunning, DependsOn: []string{"mysql"}}
	lines := serviceDetailContentLines(m, svc, 120)
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "Depends on") {
		t.Errorf("expected 'Depends on' header:\n%s", joined)
	}
	if !strings.Contains(joined, "mysql") {
		t.Errorf("expected mysql dep:\n%s", joined)
	}
}

func TestServiceDetail_DependencyShowsPresetDropIn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}

	m := NewModel("test")
	svc := &ServiceRow{Name: "phpmyadmin", State: stateRunning, DependsOn: []string{"mysql"}}
	joined := stripANSI(strings.Join(serviceDetailContentLines(m, svc, 120), "\n"))
	if !strings.Contains(joined, "mariadb") {
		t.Errorf("expected mariadb display name for mysql dep:\n%s", joined)
	}
	if strings.Contains(joined, "mariadb-11-8") {
		t.Errorf("versioned name must not appear:\n%s", joined)
	}
}

func TestServiceDetail_NoDependenciesHidesSection(t *testing.T) {
	m := NewModel("test")
	svc := &ServiceRow{Name: "redis", State: stateRunning}
	lines := serviceDetailContentLines(m, svc, 120)
	joined := stripANSI(strings.Join(lines, "\n"))
	if strings.Contains(joined, "Depends on") {
		t.Errorf("Depends on header should be hidden when empty:\n%s", joined)
	}
}

func TestServiceHints_ListTheQuickActions(t *testing.T) {
	m := NewModel("test")
	m.snap.Services = []ServiceRow{{Name: "rabbitmq", State: stateRunning, Dashboard: "http://localhost:15672"}, {Name: "redis", State: stateRunning}}
	m.switchTab(tabServices)
	m.focusMain()
	hints := stripANSI(m.renderHints(200))
	for _, want := range []string{"s start", "r restart", "P pin", "O dashboard"} {
		if !strings.Contains(hints, want) {
			t.Errorf("hints missing %q: %s", want, hints)
		}
	}
	if !strings.Contains(stripANSI(strings.Join(m.serviceHeader(m.currentService(), 120), "\n")), "http://localhost:15672") {
		t.Error("the header should show the dashboard URL")
	}
	m.svcCursor = 1
	if strings.Contains(stripANSI(m.renderHints(200)), "O dashboard") {
		t.Error("a service without a dashboard should not advertise the open action")
	}
}

func TestOpenInBrowser_ServiceDashboard(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabServices
	m.focus = paneServices
	m.snap.Services = []ServiceRow{{Name: "rabbitmq", State: stateRunning, Dashboard: "http://localhost:15672"}}
	m.svcCursor = 0
	// The platform has an opener, so a real dashboard yields a cmd (the test
	// never runs it, so no browser actually launches).
	if platform.Current.Opener != "" && m.openInBrowserCmd() == nil {
		t.Error("expected a command to open the service dashboard")
	}

	m.snap.Services = []ServiceRow{{Name: "redis", State: stateRunning}}
	if m.openInBrowserCmd() != nil {
		t.Error("a service with no dashboard should not open anything")
	}
}

func TestServiceDetail_WorkerRowRendersWorkerVariant(t *testing.T) {
	m := NewModel("test")
	svc := &ServiceRow{
		Name: "queue-acme", State: stateRunning,
		WorkerKind: "queue", WorkerSite: "acme", WorkerPath: "/home/u/Code/acme",
	}
	lines := serviceDetailContentLines(m, svc, 120)
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "kind:") || !strings.Contains(joined, "queue") {
		t.Errorf("expected worker kind row:\n%s", joined)
	}
	if !strings.Contains(joined, "site:") || !strings.Contains(joined, "acme") {
		t.Errorf("expected worker site row:\n%s", joined)
	}
	if !strings.Contains(joined, "lerd-queue-acme") {
		t.Errorf("expected unit name:\n%s", joined)
	}
	// Workers have no preset/env block, so the regular Sites-using header
	// must not appear.
	if strings.Contains(joined, "Sites using") {
		t.Errorf("worker variant should not show Sites-using:\n%s", joined)
	}
}

func TestServiceDetail_NilServiceShowsPlaceholder(t *testing.T) {
	m := NewModel("test")
	lines := serviceDetailContentLines(m, nil, 120)
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "no service selected") {
		t.Errorf("expected placeholder for nil service:\n%s", joined)
	}
}

func TestPresetSuggestionFor_KnownService(t *testing.T) {
	svc := &ServiceRow{Name: "mysql"}
	// May or may not return a string depending on whether phpmyadmin is
	// installed on the dev box; just assert the function doesn't panic
	// and that an unknown name returns "".
	_ = presetSuggestionFor(svc)
	if got := presetSuggestionFor(&ServiceRow{Name: "redis"}); got != "" {
		t.Errorf("redis has no suggestion mapping, got %q", got)
	}
	if got := presetSuggestionFor(nil); got != "" {
		t.Errorf("nil svc must return empty, got %q", got)
	}
}

// customShimService writes a custom service YAML exposing one client tool and a
// tuning override, the fixture the client-tool and tuning sections read.
func customShimService(t *testing.T, name string) {
	t.Helper()
	svc := &config.CustomService{
		Name:        name,
		Image:       "docker.io/library/redis:7",
		ClientShims: []config.ClientShim{{Name: "acme-cli", Binaries: []string{"acme-cli"}}},
		Tuning:      &config.TuningSpec{Target: "/etc/acme/acme.conf"},
	}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}
}

func TestServiceDetail_ListsClientTools(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	customShimService(t, "acmecache")

	m := NewModel("test")
	svc := &ServiceRow{Name: "acmecache", State: stateRunning, Custom: true}
	joined := stripANSI(strings.Join(serviceDetailContentLines(m, svc, 120), "\n"))
	if !strings.Contains(joined, "Client tools") {
		t.Errorf("expected a client tools section:\n%s", joined)
	}
	if !strings.Contains(joined, "acme-cli") || !strings.Contains(joined, "off") {
		t.Errorf("expected the declared tool and its state:\n%s", joined)
	}
	if !strings.Contains(joined, "space toggles the selected tool") {
		t.Errorf("expected the toggle hint in the action row:\n%s", joined)
	}
}

func TestServiceDetail_ClientToolCursorFollowsFocus(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	customShimService(t, "acmecache")

	m := NewModel("test")
	m.snap = Snapshot{Services: []ServiceRow{{Name: "acmecache", State: stateRunning, Custom: true}}}
	m.activeTab = tabServices
	m.focus = paneDetail
	_, cursor := serviceDetailContentLinesWithCursor(m, m.currentService(), 120)
	if cursor < 0 {
		t.Fatalf("expected the client-tool row to report a cursor line, got %d", cursor)
	}
	if n := m.serviceShimNavCount(); n != 1 {
		t.Fatalf("expected one toggleable client tool, got %d", n)
	}
}

func TestServiceDetail_ForeignToolIsNotToggleable(t *testing.T) {
	tools := []shims.Info{
		{Tool: "psql", Owner: "postgres"},
		{Tool: "acme-cli", Owner: "acmecache"},
	}
	nav := navigableShimRows(tools, "acmecache")
	if len(nav) != 1 || nav[0] != 1 {
		t.Fatalf("expected only the owned tool to be navigable, got %v", nav)
	}
	row := stripANSI(renderShimRow(tools[0], "acmecache", false))
	if !strings.Contains(row, "provided by postgres") {
		t.Errorf("expected the owning service on a foreign tool row: %q", row)
	}
}

func TestServiceDetail_ShowsTuningValues(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	customShimService(t, "acmecache")
	path := config.ServiceTuningFile("acmecache")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# commented out\nmaxmemory 512mb\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewModel("test")
	svc := &ServiceRow{Name: "acmecache", State: stateRunning, Custom: true}
	joined := stripANSI(strings.Join(serviceDetailContentLines(m, svc, 120), "\n"))
	if !strings.Contains(joined, "Tuning") || !strings.Contains(joined, "/etc/acme/acme.conf") {
		t.Errorf("expected the tuning section with its mount target:\n%s", joined)
	}
	if !strings.Contains(joined, "maxmemory 512mb") {
		t.Errorf("expected the set value:\n%s", joined)
	}
	if strings.Contains(joined, "commented out") {
		t.Errorf("commented lines are not in effect and should not show:\n%s", joined)
	}
	if !strings.Contains(joined, "lerd service config acmecache") {
		t.Errorf("expected the CLI edit hint:\n%s", joined)
	}
}

func TestServiceDetail_ShowsEntities(t *testing.T) {
	m := NewModel("test")
	svc := &ServiceRow{Name: "rustfs", State: stateRunning}
	m.svcEntities = map[string][]serviceEntityKind{
		"rustfs": {{
			kind:    "buckets",
			label:   "Buckets",
			columns: []serviceEntityColumn{{key: "size", label: "size", format: "bytes"}},
			rows:    []serviceops.EntityRow{{Name: "media", Values: []string{"2097152"}}},
		}},
	}
	joined := stripANSI(strings.Join(serviceDetailContentLines(m, svc, 120), "\n"))
	if !strings.Contains(joined, "Buckets") || !strings.Contains(joined, "media") {
		t.Errorf("expected the entity kind and its row:\n%s", joined)
	}
	if !strings.Contains(joined, "size 2MB") {
		t.Errorf("expected the byte column formatted:\n%s", joined)
	}
	if !strings.Contains(joined, "create and drop live in the CLI") {
		t.Errorf("expected the CLI-only note:\n%s", joined)
	}
}

func TestServiceDetail_EntitiesLoadingPlaceholder(t *testing.T) {
	m := NewModel("test")
	m.svcEntitiesLoading = "rustfs"
	svc := &ServiceRow{Name: "rustfs", State: stateRunning}
	joined := stripANSI(strings.Join(serviceDetailContentLines(m, svc, 120), "\n"))
	if !strings.Contains(joined, "Entities") || !strings.Contains(joined, "listing…") {
		t.Errorf("expected a listing placeholder while the exec is in flight:\n%s", joined)
	}
}

func TestServiceDetail_ScrollsPastTheLastClientTool(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	customShimService(t, "acmecache")

	m := NewModel("test")
	m.snap = Snapshot{Services: []ServiceRow{{Name: "acmecache", State: stateRunning, Custom: true}}}
	m.activeTab = tabServices
	m.focus = paneDetail
	// The one client tool is already selected, so the next move has to scroll
	// the pane instead of sitting on the row: tuning lives below it.
	m.moveCursor(1)
	if m.svcDetailCursor != 0 {
		t.Fatalf("cursor should stay on the only toggleable row, got %d", m.svcDetailCursor)
	}
	if m.detailScroll != 1 {
		t.Fatalf("expected the pane to scroll past the last row, got %d", m.detailScroll)
	}
	if m.followCursor {
		t.Fatal("a scroll past the cursor must not be dragged back by the follow")
	}
}
