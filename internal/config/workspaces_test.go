package config

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func wsCfg(ws ...Workspace) *GlobalConfig {
	return &GlobalConfig{Workspaces: ws}
}

func TestWorkspaceNames(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Client Work"}, Workspace{Name: "Side Projects"})
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"Client Work", "Side Projects"}) {
		t.Errorf("WorkspaceNames() = %v", got)
	}
	if got := (&GlobalConfig{}).WorkspaceNames(); len(got) != 0 {
		t.Errorf("empty config: got %v, want empty", got)
	}
}

func TestWorkspaceOfSite(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "Client Work", Sites: []string{"astrolov", "acme"}},
		Workspace{Name: "Side Projects", Sites: []string{"blog"}},
	)
	tests := map[string]string{
		"astrolov": "Client Work",
		"acme":     "Client Work",
		"blog":     "Side Projects",
		"scratch":  "",
	}
	for site, want := range tests {
		if got := cfg.WorkspaceOfSite(site); got != want {
			t.Errorf("WorkspaceOfSite(%q) = %q, want %q", site, got, want)
		}
	}
}

func TestSiteWorkspaceMap(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "Client Work", Sites: []string{"astrolov"}},
		Workspace{Name: "Empty"},
	)
	want := map[string]string{"astrolov": "Client Work"}
	if got := cfg.SiteWorkspaceMap(); !reflect.DeepEqual(got, want) {
		t.Errorf("SiteWorkspaceMap() = %v, want %v", got, want)
	}
}

func TestAddWorkspaceRules(t *testing.T) {
	tests := []struct {
		name    string
		start   []Workspace
		add     string
		wantErr error
		wantWS  []string
	}{
		{name: "creates", add: "Client Work", wantWS: []string{"Client Work"}},
		{name: "trims", add: "  Client Work  ", wantWS: []string{"Client Work"}},
		{name: "rejects empty", add: "   ", wantErr: ErrWorkspaceName},
		{
			name:    "rejects duplicate",
			start:   []Workspace{{Name: "Client Work"}},
			add:     "Client Work",
			wantErr: ErrWorkspaceExists,
			wantWS:  []string{"Client Work"},
		},
		{
			name:   "appends to the end",
			start:  []Workspace{{Name: "A"}},
			add:    "B",
			wantWS: []string{"A", "B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := wsCfg(tt.start...)
			err := cfg.addWorkspace(tt.add)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("addWorkspace(%q) err = %v, want %v", tt.add, err, tt.wantErr)
			}
			if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, tt.wantWS) && !(len(got) == 0 && len(tt.wantWS) == 0) {
				t.Errorf("names = %v, want %v", got, tt.wantWS)
			}
		})
	}
}

func TestRenameWorkspaceRules(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		wantErr  error
		wantWS   []string
	}{
		{name: "renames in place", old: "A", new: "C", wantWS: []string{"C", "B"}},
		{name: "trims", old: "A", new: "  C  ", wantWS: []string{"C", "B"}},
		{name: "missing", old: "Z", new: "C", wantErr: ErrWorkspaceNotFound, wantWS: []string{"A", "B"}},
		{name: "collision", old: "A", new: "B", wantErr: ErrWorkspaceExists, wantWS: []string{"A", "B"}},
		{name: "empty new name", old: "A", new: " ", wantErr: ErrWorkspaceName, wantWS: []string{"A", "B"}},
		{name: "same name is a no-op", old: "A", new: "A", wantWS: []string{"A", "B"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := wsCfg(Workspace{Name: "A", Sites: []string{"one"}}, Workspace{Name: "B"})
			err := cfg.renameWorkspace(tt.old, tt.new)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("renameWorkspace err = %v, want %v", err, tt.wantErr)
			}
			if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, tt.wantWS) {
				t.Errorf("names = %v, want %v", got, tt.wantWS)
			}
			if tt.wantErr == nil && len(cfg.Workspaces[0].Sites) != 1 {
				t.Error("rename dropped the workspace members")
			}
		})
	}
}

func TestDeleteWorkspaceUngroupsMembers(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "A", Sites: []string{"one", "two"}},
		Workspace{Name: "B", Sites: []string{"three"}},
	)
	cfg.deleteWorkspace("A")
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"B"}) {
		t.Fatalf("names = %v, want [B]", got)
	}
	for _, site := range []string{"one", "two"} {
		if ws := cfg.WorkspaceOfSite(site); ws != "" {
			t.Errorf("site %q still in workspace %q, want ungrouped", site, ws)
		}
	}
	if cfg.WorkspaceOfSite("three") != "B" {
		t.Error("deleting A disturbed B's members")
	}
}

func TestDeleteWorkspaceUnknownIsNoOp(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "A"})
	cfg.deleteWorkspace("Z")
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("names = %v, want [A]", got)
	}
}

func TestAssignSites(t *testing.T) {
	tests := []struct {
		name      string
		sites     []string
		workspace string
		create    bool
		wantErr   error
		wantOf    map[string]string
		wantNames []string
	}{
		{
			name: "moves between workspaces", sites: []string{"one"}, workspace: "B",
			wantOf:    map[string]string{"one": "B", "two": "A"},
			wantNames: []string{"A", "B"},
		},
		{
			name: "moves several at once", sites: []string{"one", "two"}, workspace: "B",
			wantOf:    map[string]string{"one": "B", "two": "B"},
			wantNames: []string{"A", "B"},
		},
		{
			name: "empty workspace ungroups", sites: []string{"one"}, workspace: "",
			wantOf:    map[string]string{"one": "", "two": "A"},
			wantNames: []string{"A", "B"},
		},
		{
			name: "creates the target when asked", sites: []string{"one"}, workspace: "New", create: true,
			wantOf:    map[string]string{"one": "New"},
			wantNames: []string{"A", "B", "New"},
		},
		{
			name: "unknown target without create", sites: []string{"one"}, workspace: "New",
			wantErr:   ErrWorkspaceNotFound,
			wantOf:    map[string]string{"one": "A"},
			wantNames: []string{"A", "B"},
		},
		{
			name: "assigning to the current workspace is idempotent", sites: []string{"one"}, workspace: "A",
			wantOf:    map[string]string{"one": "A", "two": "A"},
			wantNames: []string{"A", "B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := wsCfg(Workspace{Name: "A", Sites: []string{"one", "two"}}, Workspace{Name: "B"})
			err := cfg.assignSites(tt.sites, tt.workspace, tt.create)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("assignSites err = %v, want %v", err, tt.wantErr)
			}
			for site, want := range tt.wantOf {
				if got := cfg.WorkspaceOfSite(site); got != want {
					t.Errorf("WorkspaceOfSite(%q) = %q, want %q", site, got, want)
				}
			}
			if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, tt.wantNames) {
				t.Errorf("names = %v, want %v", got, tt.wantNames)
			}
		})
	}
}

func TestAssignSitesNeverDuplicatesAMember(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "A", Sites: []string{"one"}})
	if err := cfg.assignSites([]string{"one", "one"}, "A", false); err != nil {
		t.Fatalf("assignSites: %v", err)
	}
	if got := cfg.Workspaces[0].Sites; !reflect.DeepEqual(got, []string{"one"}) {
		t.Errorf("sites = %v, want [one]", got)
	}
}

func TestMoveWorkspaceTo(t *testing.T) {
	tests := []struct {
		name    string
		move    string
		pos     int
		wantErr error
		want    []string
	}{
		{name: "to the front", move: "C", pos: 0, want: []string{"C", "A", "B"}},
		{name: "to the middle", move: "A", pos: 1, want: []string{"B", "A", "C"}},
		{name: "to the end", move: "A", pos: 2, want: []string{"B", "C", "A"}},
		{name: "clamps past the end", move: "A", pos: 99, want: []string{"B", "C", "A"}},
		{name: "clamps below zero", move: "C", pos: -5, want: []string{"C", "A", "B"}},
		{name: "unknown workspace", move: "Z", pos: 0, wantErr: ErrWorkspaceNotFound, want: []string{"A", "B", "C"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := wsCfg(Workspace{Name: "A"}, Workspace{Name: "B"}, Workspace{Name: "C"})
			err := cfg.moveWorkspace(tt.move, tt.pos)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("moveWorkspace err = %v, want %v", err, tt.wantErr)
			}
			if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("names = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPruneWorkspaceSites(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "A", Sites: []string{"live", "gone"}},
		Workspace{Name: "Empty"},
	)
	cfg.pruneWorkspaceSites(map[string]bool{"live": true})
	if got := cfg.Workspaces[0].Sites; !reflect.DeepEqual(got, []string{"live"}) {
		t.Errorf("sites = %v, want [live]", got)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"A", "Empty"}) {
		t.Errorf("prune removed an empty workspace: %v", got)
	}
}

func TestSetWorkspaceLayoutRules(t *testing.T) {
	tests := []struct {
		name    string
		layout  []Workspace
		wantErr error
	}{
		{name: "accepts a full layout", layout: []Workspace{{Name: "B", Sites: []string{"one"}}, {Name: "A"}}},
		{name: "rejects an empty name", layout: []Workspace{{Name: " "}}, wantErr: ErrWorkspaceName},
		{name: "rejects duplicates", layout: []Workspace{{Name: "A"}, {Name: "A"}}, wantErr: ErrWorkspaceExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := wsCfg(Workspace{Name: "A", Sites: []string{"one"}})
			err := cfg.setWorkspaceLayout(tt.layout)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("setWorkspaceLayout err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"B", "A"}) {
				t.Errorf("names = %v, want [B A]", got)
			}
			if cfg.WorkspaceOfSite("one") != "B" {
				t.Error("layout did not move the site")
			}
		})
	}
}

func TestSetWorkspaceLayoutDropsADuplicatedSite(t *testing.T) {
	cfg := wsCfg()
	err := cfg.setWorkspaceLayout([]Workspace{
		{Name: "A", Sites: []string{"one"}},
		{Name: "B", Sites: []string{"one", "two"}},
	})
	if err != nil {
		t.Fatalf("setWorkspaceLayout: %v", err)
	}
	if got := cfg.WorkspaceOfSite("one"); got != "A" {
		t.Errorf("WorkspaceOfSite(one) = %q, want A (first wins)", got)
	}
	if got := cfg.Workspaces[1].Sites; !reflect.DeepEqual(got, []string{"two"}) {
		t.Errorf("B sites = %v, want [two]", got)
	}
}

// ── persistence ───────────────────────────────────────────────────────────────

func TestWorkspacesSurviveASaveLoadRoundTrip(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Workspaces = []Workspace{
		{Name: "Client Work", Sites: []string{"astrolov", "acme"}},
		{Name: "Empty"},
	}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !reflect.DeepEqual(got.Workspaces, cfg.Workspaces) {
		t.Errorf("workspaces = %+v, want %+v", got.Workspaces, cfg.Workspaces)
	}
	if got.PHP.DefaultVersion == "" {
		t.Error("defaults merge lost the PHP version")
	}
}

func TestWorkspacesAbsentFromConfigStayAbsent(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	data, err := os.ReadFile(GlobalConfigFile())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "workspaces:") {
		t.Error("an install with no workspaces wrote a workspaces: key")
	}
}

// cloneGlobalConfig must deep-copy the workspaces, or a caller mutating the
// clone returned by LoadGlobal would corrupt the cached config.
func TestCloneGlobalConfigDeepCopiesWorkspaces(t *testing.T) {
	in := wsCfg(Workspace{Name: "A", Sites: []string{"one"}})
	out := cloneGlobalConfig(in)
	out.Workspaces[0].Name = "mutated"
	out.Workspaces[0].Sites[0] = "mutated"
	if in.Workspaces[0].Name != "A" || in.Workspaces[0].Sites[0] != "one" {
		t.Errorf("clone aliases the source: %+v", in.Workspaces[0])
	}
}

// ── the exported, locking wrappers ────────────────────────────────────────────

func TestWorkspaceWrappersPersist(t *testing.T) {
	setConfigDir(t)

	if err := AddWorkspace("Client Work"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	if err := AddWorkspace("Client Work"); !errors.Is(err, ErrWorkspaceExists) {
		t.Fatalf("duplicate AddWorkspace err = %v, want ErrWorkspaceExists", err)
	}
	if err := AssignSiteWorkspace([]string{"astrolov"}, "Client Work", false); err != nil {
		t.Fatalf("AssignSiteWorkspace: %v", err)
	}
	if err := RenameWorkspace("Client Work", "Clients"); err != nil {
		t.Fatalf("RenameWorkspace: %v", err)
	}
	if err := MoveWorkspace("Clients", 0); err != nil {
		t.Fatalf("MoveWorkspace: %v", err)
	}

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if got := cfg.WorkspaceOfSite("astrolov"); got != "Clients" {
		t.Errorf("WorkspaceOfSite(astrolov) = %q, want Clients", got)
	}

	if err := DeleteWorkspace("Clients"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	cfg, err = LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if len(cfg.Workspaces) != 0 {
		t.Errorf("workspaces = %+v, want none", cfg.Workspaces)
	}
}

func TestAssignSiteWorkspaceCreatesTheTarget(t *testing.T) {
	setConfigDir(t)
	if err := AssignSiteWorkspace([]string{"blog"}, "Side Projects", true); err != nil {
		t.Fatalf("AssignSiteWorkspace: %v", err)
	}
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if got := cfg.WorkspaceOfSite("blog"); got != "Side Projects" {
		t.Errorf("WorkspaceOfSite(blog) = %q, want Side Projects", got)
	}
}

func TestSetWorkspaceLayoutPersists(t *testing.T) {
	setConfigDir(t)
	layout := []Workspace{{Name: "B", Sites: []string{"two"}}, {Name: "A", Sites: []string{"one"}}}
	if err := SetWorkspaceLayout(layout); err != nil {
		t.Fatalf("SetWorkspaceLayout: %v", err)
	}
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !reflect.DeepEqual(cfg.Workspaces, layout) {
		t.Errorf("workspaces = %+v, want %+v", cfg.Workspaces, layout)
	}
}

// ListWorkspaces drops names of sites that are no longer in the registry, but
// leaves the config on disk untouched.
func TestListWorkspacesPrunesStaleSiteNames(t *testing.T) {
	setConfigDir(t)
	if err := SetWorkspaceLayout([]Workspace{{Name: "A", Sites: []string{"live", "gone"}}}); err != nil {
		t.Fatalf("SetWorkspaceLayout: %v", err)
	}
	if err := AddSite(Site{Name: "live", Path: t.TempDir(), Domains: []string{"live.test"}}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	got, err := ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	want := []Workspace{{Name: "A", Sites: []string{"live"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListWorkspaces() = %+v, want %+v", got, want)
	}

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if len(cfg.Workspaces[0].Sites) != 2 {
		t.Error("ListWorkspaces rewrote the config on disk")
	}
}
