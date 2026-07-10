package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// workspaceEnv points the global config and the site registry at temp dirs and
// registers a couple of sites to assign around.
func workspaceEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	for _, name := range []string{"astrolov", "blog"} {
		site := config.Site{Name: name, Path: t.TempDir(), Domains: []string{name + ".test"}}
		if err := config.AddSite(site); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceAddRenameRemove(t *testing.T) {
	workspaceEnv(t)

	if err := runWorkspaceAdd(nil, []string{"Client Work"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := runWorkspaceAdd(nil, []string{"Client Work"}); !errors.Is(err, config.ErrWorkspaceExists) {
		t.Fatalf("duplicate add err = %v, want ErrWorkspaceExists", err)
	}
	if err := runWorkspaceRename(nil, []string{"Client Work", "Clients"}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].Name != "Clients" {
		t.Fatalf("workspaces = %+v, want one named Clients", cfg.Workspaces)
	}

	if err := runWorkspaceRemove(nil, []string{"Clients"}); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if err := runWorkspaceRemove(nil, []string{"Clients"}); !errors.Is(err, config.ErrWorkspaceNotFound) {
		t.Fatalf("rm of a missing workspace err = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestWorkspaceAssignByNameAndDomain(t *testing.T) {
	workspaceEnv(t)
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}

	if err := runWorkspaceAssign(nil, []string{"astrolov", "Clients"}); err != nil {
		t.Fatalf("assign by name: %v", err)
	}
	if err := runWorkspaceAssign(nil, []string{"blog.test", "Clients"}); err != nil {
		t.Fatalf("assign by domain: %v", err)
	}
	cfg, _ := config.LoadGlobal()
	for _, site := range []string{"astrolov", "blog"} {
		if got := cfg.WorkspaceOfSite(site); got != "Clients" {
			t.Errorf("WorkspaceOfSite(%s) = %q, want Clients", site, got)
		}
	}
}

func TestWorkspaceAssignNoneUngroups(t *testing.T) {
	workspaceEnv(t)
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceAssign(nil, []string{"astrolov", "Clients"}); err != nil {
		t.Fatal(err)
	}

	if err := runWorkspaceAssign(nil, []string{"astrolov", "none"}); err != nil {
		t.Fatalf("assign none: %v", err)
	}
	cfg, _ := config.LoadGlobal()
	if got := cfg.WorkspaceOfSite("astrolov"); got != "" {
		t.Errorf("WorkspaceOfSite(astrolov) = %q, want ungrouped", got)
	}
	if len(cfg.Workspaces) != 1 {
		t.Error("ungrouping a site removed the workspace")
	}
}

func TestWorkspaceAssignRejectsAnUnknownSite(t *testing.T) {
	workspaceEnv(t)
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}
	err := runWorkspaceAssign(nil, []string{"nope", "Clients"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want a not-found error", err)
	}
}

func TestWorkspaceAssignRejectsAnUnknownWorkspace(t *testing.T) {
	workspaceEnv(t)
	if err := runWorkspaceAssign(nil, []string{"astrolov", "Nope"}); !errors.Is(err, config.ErrWorkspaceNotFound) {
		t.Errorf("err = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestWorkspaceMove(t *testing.T) {
	workspaceEnv(t)
	for _, name := range []string{"A", "B", "C"} {
		if err := runWorkspaceAdd(nil, []string{name}); err != nil {
			t.Fatal(err)
		}
	}

	if err := runWorkspaceMove(nil, []string{"C", "0"}); err != nil {
		t.Fatalf("move: %v", err)
	}
	cfg, _ := config.LoadGlobal()
	if got := cfg.WorkspaceNames(); strings.Join(got, ",") != "C,A,B" {
		t.Errorf("names = %v, want [C A B]", got)
	}

	if err := runWorkspaceMove(nil, []string{"C", "not-a-number"}); err == nil {
		t.Error("expected a parse error for a non-numeric position")
	}
}

func TestWorkspaceListShowsMembersAndUngrouped(t *testing.T) {
	workspaceEnv(t)
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceAssign(nil, []string{"astrolov", "Clients"}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runWorkspaceList(nil, nil); err != nil {
			t.Errorf("list: %v", err)
		}
	})
	for _, want := range []string{"Clients", "  astrolov", "Ungrouped", "  blog"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

// addSecondary registers a group secondary of the given main.
func addSecondary(t *testing.T, name, main string) {
	t.Helper()
	site := config.Site{
		Name: name, Path: t.TempDir(), Domains: []string{name + "." + main + ".test"},
		Group: main, GroupSubdomain: name,
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
}

// A secondary displays in its main's workspace, so it has no membership of its
// own to set and assigning it directly would write state nothing ever reads.
func TestWorkspaceAssignRejectsAGroupSecondary(t *testing.T) {
	workspaceEnv(t)
	addSecondary(t, "admin", "astrolov")
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}

	err := runWorkspaceAssign(nil, []string{"admin", "Clients"})
	if err == nil || !strings.Contains(err.Error(), "astrolov") {
		t.Fatalf("err = %v, want a pointer at the group main", err)
	}
	cfg, _ := config.LoadGlobal()
	if got := cfg.WorkspaceOfSite("admin"); got != "" {
		t.Errorf("admin was written into %q", got)
	}
}

// "none" is the sentinel that ungroups a site, and the UI labels the ungrouped
// option "None", so a workspace may not take the name.
func TestWorkspaceAddRejectsTheReservedName(t *testing.T) {
	workspaceEnv(t)
	for _, name := range []string{"none", "None", " NONE "} {
		if err := runWorkspaceAdd(nil, []string{name}); !errors.Is(err, config.ErrWorkspaceReserved) {
			t.Errorf("add %q err = %v, want ErrWorkspaceReserved", name, err)
		}
	}
}

// A secondary is listed under its main's workspace, matching the sidebar.
func TestWorkspaceListNestsSecondariesUnderTheirMain(t *testing.T) {
	workspaceEnv(t)
	addSecondary(t, "admin", "astrolov")
	if err := runWorkspaceAdd(nil, []string{"Clients"}); err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceAssign(nil, []string{"astrolov", "Clients"}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runWorkspaceList(nil, nil); err != nil {
			t.Errorf("list: %v", err)
		}
	})
	clients := strings.Index(out, "Clients")
	admin := strings.Index(out, "admin")
	ungrouped := strings.Index(out, "Ungrouped")
	if clients < 0 || admin < 0 || ungrouped < 0 {
		t.Fatalf("list output missing a section:\n%s", out)
	}
	if !(clients < admin && admin < ungrouped) {
		t.Errorf("admin should sit under Clients, not below Ungrouped:\n%s", out)
	}
}

func TestWorkspaceListWithoutWorkspaces(t *testing.T) {
	workspaceEnv(t)
	out := captureStdout(t, func() {
		if err := runWorkspaceList(nil, nil); err != nil {
			t.Errorf("list: %v", err)
		}
	})
	if !strings.Contains(out, "No workspaces.") {
		t.Errorf("output = %q, want the empty notice", out)
	}
}
