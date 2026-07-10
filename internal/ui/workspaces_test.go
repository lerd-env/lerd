package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

// workspaceEnv points the global config and the site registry at temp dirs.
func workspaceEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
}

func postWorkspace(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	if path == "/api/workspaces" {
		handleWorkspaces(rec, req)
	} else {
		handleWorkspaceRoutes(rec, req)
	}
	return rec
}

func wantOK(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok=true, got %s", rec.Body.String())
	}
}

func TestHandleWorkspacesCreateAndList(t *testing.T) {
	workspaceEnv(t)

	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "Client Work"}))
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "Side Projects"}))

	rec := httptest.NewRecorder()
	handleWorkspaces(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	var got []WorkspaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	want := []WorkspaceResponse{
		{Name: "Client Work", Sites: []string{}},
		{Name: "Side Projects", Sites: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GET /api/workspaces = %+v, want %+v", got, want)
	}
}

func TestHandleWorkspacesCreateRejectsADuplicate(t *testing.T) {
	workspaceEnv(t)
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "A"}))

	rec := postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "A"})
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Errorf("expected a duplicate error, got %s", rec.Body.String())
	}
}

func TestHandleWorkspaceRename(t *testing.T) {
	workspaceEnv(t)
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "A"}))
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces/rename", workspaceRenameRequest{Old: "A", New: "B"}))

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"B"}) {
		t.Errorf("names = %v, want [B]", got)
	}
}

func TestHandleWorkspaceDeleteUngroupsItsSites(t *testing.T) {
	workspaceEnv(t)
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces", workspaceCreateRequest{Name: "A"}))
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces/assign", workspaceAssignRequest{Sites: []string{"blog"}, Workspace: "A"}))
	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces/delete", workspaceCreateRequest{Name: "A"}))

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 0 {
		t.Errorf("workspaces = %+v, want none", cfg.Workspaces)
	}
	if ws := cfg.WorkspaceOfSite("blog"); ws != "" {
		t.Errorf("blog still in %q, want ungrouped", ws)
	}
}

func TestHandleWorkspaceAssign(t *testing.T) {
	workspaceEnv(t)

	rec := postWorkspace(t, http.MethodPost, "/api/workspaces/assign", workspaceAssignRequest{Sites: []string{"blog"}, Workspace: "New"})
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("assign to a missing workspace should fail, got %s", rec.Body.String())
	}

	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces/assign", workspaceAssignRequest{Sites: []string{"blog"}, Workspace: "New", Create: true}))
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceOfSite("blog"); got != "New" {
		t.Errorf("WorkspaceOfSite(blog) = %q, want New", got)
	}

	wantOK(t, postWorkspace(t, http.MethodPost, "/api/workspaces/assign", workspaceAssignRequest{Sites: []string{"blog"}, Workspace: ""}))
	cfg, _ = config.LoadGlobal()
	if got := cfg.WorkspaceOfSite("blog"); got != "" {
		t.Errorf("WorkspaceOfSite(blog) = %q, want ungrouped", got)
	}
}

func TestHandleWorkspaceLayoutWritesOrderAndMembership(t *testing.T) {
	workspaceEnv(t)
	for _, name := range []string{"one", "two"} {
		if err := config.AddSite(config.Site{Name: name, Path: t.TempDir(), Domains: []string{name + ".test"}}); err != nil {
			t.Fatal(err)
		}
	}

	body := workspaceLayoutRequest{
		Workspaces: []WorkspaceResponse{
			{Name: "B", Sites: []string{"two"}},
			{Name: "A", Sites: []string{"one"}},
		},
		SiteOrder: []string{"two", "one"},
	}
	wantOK(t, postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body))

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"B", "A"}) {
		t.Errorf("names = %v, want [B A]", got)
	}
	if got := cfg.WorkspaceOfSite("two"); got != "B" {
		t.Errorf("WorkspaceOfSite(two) = %q, want B", got)
	}
	reg, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Sites[0].Name != "two" || reg.Sites[1].Name != "one" {
		t.Errorf("registry order = %q %q, want two one", reg.Sites[0].Name, reg.Sites[1].Name)
	}
}

// Omitting site_order means the drag only moved a workspace, so the registry
// must be left exactly as it was.
func TestHandleWorkspaceLayoutWithoutSiteOrderLeavesTheRegistryAlone(t *testing.T) {
	workspaceEnv(t)
	for _, name := range []string{"one", "two"} {
		if err := config.AddSite(config.Site{Name: name, Path: t.TempDir(), Domains: []string{name + ".test"}}); err != nil {
			t.Fatal(err)
		}
	}

	body := workspaceLayoutRequest{Workspaces: []WorkspaceResponse{{Name: "A", Sites: []string{"two"}}}}
	wantOK(t, postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body))

	reg, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Sites[0].Name != "one" || reg.Sites[1].Name != "two" {
		t.Errorf("registry order = %q %q, want one two", reg.Sites[0].Name, reg.Sites[1].Name)
	}
}

// A client that drags before it hears about a workspace created elsewhere must
// not wipe it out, so an omitted workspace survives with its members.
func TestHandleWorkspaceLayoutKeepsAWorkspaceTheClientDidNotSend(t *testing.T) {
	workspaceEnv(t)
	if err := config.SetWorkspaceLayout([]config.Workspace{
		{Name: "Known", Sites: []string{"one"}},
		{Name: "CreatedElsewhere", Sites: []string{"two"}},
	}); err != nil {
		t.Fatal(err)
	}

	body := workspaceLayoutRequest{Workspaces: []WorkspaceResponse{{Name: "Known", Sites: []string{"one"}}}}
	wantOK(t, postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body))

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"Known", "CreatedElsewhere"}) {
		t.Errorf("names = %v, want [Known CreatedElsewhere]", got)
	}
	if got := cfg.WorkspaceOfSite("two"); got != "CreatedElsewhere" {
		t.Errorf("WorkspaceOfSite(two) = %q, want CreatedElsewhere", got)
	}
}

// Moving a site out of a workspace the client didn't send still takes effect:
// the sent workspaces are listed first, and the first listing of a site wins.
func TestHandleWorkspaceLayoutMovesASiteOutOfAnOmittedWorkspace(t *testing.T) {
	workspaceEnv(t)
	if err := config.SetWorkspaceLayout([]config.Workspace{
		{Name: "Known"},
		{Name: "CreatedElsewhere", Sites: []string{"two"}},
	}); err != nil {
		t.Fatal(err)
	}

	body := workspaceLayoutRequest{Workspaces: []WorkspaceResponse{{Name: "Known", Sites: []string{"two"}}}}
	wantOK(t, postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body))

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceOfSite("two"); got != "Known" {
		t.Errorf("WorkspaceOfSite(two) = %q, want Known", got)
	}
}

func TestHandleWorkspaceLayoutRejectsADuplicateName(t *testing.T) {
	workspaceEnv(t)
	if err := config.AddWorkspace("Keep"); err != nil {
		t.Fatal(err)
	}

	body := workspaceLayoutRequest{Workspaces: []WorkspaceResponse{{Name: "A"}, {Name: "A"}}}
	rec := postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body)
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("expected a duplicate error, got %s", rec.Body.String())
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"Keep"}) {
		t.Errorf("a rejected layout changed the config: %v", got)
	}
}

// When the registry write fails the config goes back and the caller hears why
// the reorder failed, not why the restore did or didn't.
func TestHandleWorkspaceLayoutRollsBackAndReportsTheReorderError(t *testing.T) {
	workspaceEnv(t)
	if err := config.AddSite(config.Site{Name: "one", Path: t.TempDir(), Domains: []string{"one.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := config.SetWorkspaceLayout([]config.Workspace{{Name: "Before", Sites: []string{"one"}}}); err != nil {
		t.Fatal(err)
	}

	// Make sites.yaml unwritable so ReorderSites fails after the config write.
	if err := os.Chmod(config.DataDir(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(config.DataDir(), 0o755) })

	body := workspaceLayoutRequest{
		Workspaces: []WorkspaceResponse{{Name: "After", Sites: []string{"one"}}},
		SiteOrder:  []string{"one"},
	}
	rec := postWorkspace(t, http.MethodPut, "/api/workspaces/layout", body)
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("expected a failure, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "permission denied") {
		t.Errorf("expected the reorder error, got %s", rec.Body.String())
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.WorkspaceNames(); !reflect.DeepEqual(got, []string{"Before"}) {
		t.Errorf("names = %v, want the pre-write layout [Before]", got)
	}
}

func TestWorkspaceRoutesRejectTheWrongMethod(t *testing.T) {
	workspaceEnv(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/workspaces/assign"},
		{http.MethodPost, "/api/workspaces/layout"},
		{http.MethodGet, "/api/workspaces/nope"},
	} {
		rec := postWorkspace(t, tc.method, tc.path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestResolveSiteWorkspace(t *testing.T) {
	groupMainName := map[string]string{"astrolov": "astrolov"}
	siteWorkspace := map[string]string{"astrolov": "Client Work", "admin": "Side Projects"}

	tests := []struct {
		name string
		site siteinfo.EnrichedSite
		want string
	}{
		{
			name: "a standalone site uses its own membership",
			site: siteinfo.EnrichedSite{Name: "admin"},
			want: "Side Projects",
		},
		{
			name: "a secondary follows its main, not its own membership",
			site: siteinfo.EnrichedSite{Name: "admin", Group: "astrolov", GroupSubdomain: "admin"},
			want: "Client Work",
		},
		{
			name: "a secondary whose main is missing falls back to its own",
			site: siteinfo.EnrichedSite{Name: "admin", Group: "gone", GroupSubdomain: "admin"},
			want: "Side Projects",
		},
		{
			name: "an ungrouped site reports nothing",
			site: siteinfo.EnrichedSite{Name: "scratch"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSiteWorkspace(tt.site, groupMainName, siteWorkspace); got != tt.want {
				t.Errorf("resolveSiteWorkspace() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildStatusReportsWorkspacesInOrder(t *testing.T) {
	workspaceEnv(t)
	for _, name := range []string{"B", "A"} {
		if err := config.AddWorkspace(name); err != nil {
			t.Fatal(err)
		}
	}
	if got := buildStatus().Workspaces; !reflect.DeepEqual(got, []string{"B", "A"}) {
		t.Errorf("status workspaces = %v, want [B A]", got)
	}
}

func TestBuildStatusWorkspacesIsNeverNull(t *testing.T) {
	workspaceEnv(t)
	raw, err := json.Marshal(buildStatus())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"workspaces":[]`) {
		t.Errorf("expected an empty array, got %s", raw)
	}
}
