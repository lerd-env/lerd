package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeSnippet(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Snippets come from the project's .lerd/tinker/snippets directory, one .php
// file each, labelled by filename, sorted by label, content loaded verbatim.
func TestListTinkerSnippets_projectDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sitePath := t.TempDir()
	dir := filepath.Join(sitePath, ".lerd", "tinker", "snippets")
	writeSnippet(t, dir, "reset-orders.php", "Order::query()->delete();\n")
	writeSnippet(t, dir, "count-users.php", "User::count();\n")
	writeSnippet(t, dir, "notes.txt", "not a snippet")
	writeSnippet(t, dir, ".hidden.php", "skip me")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := listTinkerSnippets(sitePath)
	if len(got) != 2 {
		t.Fatalf("got %d snippets, want 2: %+v", len(got), got)
	}
	if got[0].Name != "count-users.php" || got[0].Label != "count-users" {
		t.Errorf("first snippet: %+v", got[0])
	}
	if got[1].Name != "reset-orders.php" || got[1].Label != "reset-orders" {
		t.Errorf("second snippet: %+v", got[1])
	}
	if got[0].Source != "project" || got[1].Source != "project" {
		t.Errorf("sources: %q, %q", got[0].Source, got[1].Source)
	}
	if got[1].Content != "Order::query()->delete();\n" {
		t.Errorf("content: %q", got[1].Content)
	}
}

// A // @name header in the first lines overrides the filename as label.
func TestListTinkerSnippets_nameHeader(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sitePath := t.TempDir()
	dir := filepath.Join(sitePath, ".lerd", "tinker", "snippets")
	writeSnippet(t, dir, "fix-1234.php", "<?php\n// @name Reset demo data\nOrder::truncate();\n")

	got := listTinkerSnippets(sitePath)
	if len(got) != 1 {
		t.Fatalf("got %d snippets, want 1", len(got))
	}
	if got[0].Label != "Reset demo data" {
		t.Errorf("label: %q, want %q", got[0].Label, "Reset demo data")
	}
	if !strings.Contains(got[0].Content, "@name") {
		t.Errorf("content must stay verbatim, got %q", got[0].Content)
	}
}

// The header accepts both the spaced and the colon form, but a run-on word
// after @name is not a header.
func TestSnippetLabel_nameHeaderVariants(t *testing.T) {
	cases := map[string]string{
		"// @name Spaced form\n1;":                         "Spaced form",
		"// @name: Colon form\n1;":                         "Colon form",
		"// @name:Tight colon\n1;":                         "Tight colon",
		"// @namesake comment\n1;":                         "file",
		"$x = 1; // @name Late\n1;":                        "file",
		strings.Repeat("\n", 12) + "// @name Too deep\n1;": "file",
	}
	for content, want := range cases {
		if got := snippetLabel("file.php", content); got != want {
			t.Errorf("snippetLabel(%q) = %q, want %q", content, got, want)
		}
	}
}

// .tinkerwell/snippets is honoured so existing Tinkerwell users get their
// snippets for free; they list after the project's own.
func TestListTinkerSnippets_tinkerwellDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sitePath := t.TempDir()
	writeSnippet(t, filepath.Join(sitePath, ".lerd", "tinker", "snippets"), "zz.php", "1;")
	writeSnippet(t, filepath.Join(sitePath, ".tinkerwell", "snippets"), "aa.php", "2;")

	got := listTinkerSnippets(sitePath)
	if len(got) != 2 {
		t.Fatalf("got %d snippets, want 2", len(got))
	}
	if got[0].Source != "project" || got[1].Source != "tinkerwell" {
		t.Errorf("order: %q then %q, want project then tinkerwell", got[0].Source, got[1].Source)
	}
}

// Personal snippets in ~/.config/lerd/tinker/snippets follow the user across
// every site and list last.
func TestListTinkerSnippets_globalDir(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	sitePath := t.TempDir()
	writeSnippet(t, filepath.Join(sitePath, ".lerd", "tinker", "snippets"), "local.php", "1;")
	writeSnippet(t, filepath.Join(cfgHome, "lerd", "tinker", "snippets"), "everywhere.php", "2;")

	got := listTinkerSnippets(sitePath)
	if len(got) != 2 {
		t.Fatalf("got %d snippets, want 2", len(got))
	}
	if got[1].Source != "global" || got[1].Name != "everywhere.php" {
		t.Errorf("last snippet: %+v", got[1])
	}
}

// Files beyond the tinker request cap cannot run anyway, so they are skipped
// rather than truncated.
func TestListTinkerSnippets_skipsOversized(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sitePath := t.TempDir()
	dir := filepath.Join(sitePath, ".lerd", "tinker", "snippets")
	writeSnippet(t, dir, "huge.php", strings.Repeat("a", 65*1024))
	writeSnippet(t, dir, "ok.php", "1;")

	got := listTinkerSnippets(sitePath)
	if len(got) != 1 || got[0].Name != "ok.php" {
		t.Fatalf("got %+v, want only ok.php", got)
	}
}

// GET /api/sites/{domain}/tinker:snippets serves the list as JSON; a site
// with no snippet directories answers an empty array, never null.
func TestHandleSiteTinkerSnippets_http(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	writeSnippet(t, filepath.Join(sitePath, ".lerd", "tinker", "snippets"),
		"seed.php", "// @name Seed lookups\nSeeder::run();\n")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/tinker:snippets", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got []tinkerSnippet
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "Seed lookups" || got[0].Source != "project" {
		t.Fatalf("got %+v", got)
	}

	// A snippet-less site still answers a JSON array.
	empty := t.TempDir()
	if err := config.AddSite(config.Site{Name: "bare", Path: empty, Domains: []string{"bare.test"}}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/sites/bare.test/tinker:snippets", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	handleSiteAction(rec, req)
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("empty list body: %q, want []", body)
	}

	// Only GET, POST and DELETE are served.
	req = httptest.NewRequest(http.MethodPut, "/api/sites/acme.test/tinker:snippets", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT status %d, want 405", rec.Code)
	}
}

func postSnippet(t *testing.T, domain, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+domain+"/tinker:snippets", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	return rec
}

// POST saves the editor contents as a snippet file; the .php extension is
// appended when missing and the refreshed list comes back for the picker.
func TestSaveTinkerSnippet_http(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := postSnippet(t, "acme.test", `{"name":"Count users","source":"project","content":"User::count();\n"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	saved := filepath.Join(sitePath, ".lerd", "tinker", "snippets", "Count users.php")
	if b, err := os.ReadFile(saved); err != nil || string(b) != "User::count();\n" {
		t.Fatalf("saved file: %v %q", err, b)
	}
	var list []tinkerSnippet
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Label != "Count users" || list[0].Source != "project" {
		t.Fatalf("refreshed list: %+v", list)
	}

	// Global snippets land in the config dir and overwrite in place.
	rec = postSnippet(t, "acme.test", `{"name":"whoami.php","source":"global","content":"1;"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("global status %d: %s", rec.Code, rec.Body.String())
	}
	rec = postSnippet(t, "acme.test", `{"name":"whoami.php","source":"global","content":"2;"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("overwrite status %d: %s", rec.Code, rec.Body.String())
	}
	globalFile := filepath.Join(cfgHome, "lerd", "tinker", "snippets", "whoami.php")
	if b, _ := os.ReadFile(globalFile); string(b) != "2;" {
		t.Fatalf("overwrite content: %q", b)
	}
}

// A ?branch= query targets the worktree checkout for listing and saving, and
// an unknown branch answers 404 rather than falling back to the parent site.
func TestTinkerSnippets_branchTargetsWorktree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath, checkoutPath := setupWorktreeFixture(t, "feat-a")
	writeSnippet(t, filepath.Join(sitePath, ".lerd", "tinker", "snippets"), "parent.php", "1;")
	writeSnippet(t, filepath.Join(checkoutPath, ".lerd", "tinker", "snippets"), "worktree.php", "2;")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	get := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/tinker:snippets"+query, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		handleSiteAction(rec, req)
		return rec
	}

	rec := get("?branch=feat-a")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var list []tinkerSnippet
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "worktree.php" {
		t.Fatalf("worktree list: %+v", list)
	}

	if rec := get("?branch=no-such-branch"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown branch: status %d, want 404", rec.Code)
	}

	// Saving with the branch set lands in the worktree checkout, not the parent.
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/tinker:snippets?branch=feat-a",
		strings.NewReader(`{"name":"saved","source":"project","content":"3;"}`))
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(checkoutPath, ".lerd", "tinker", "snippets", "saved.php")); err != nil {
		t.Errorf("saved file not in worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sitePath, ".lerd", "tinker", "snippets", "saved.php")); !os.IsNotExist(err) {
		t.Error("save leaked into the parent site")
	}
}

// Saving validates the target: filenames cannot traverse, the tinkerwell dir
// stays read-only, and empty content is refused.
func TestSaveTinkerSnippet_rejects(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		`{"name":"../escape","source":"project","content":"1;"}`,
		`{"name":"a/b","source":"project","content":"1;"}`,
		`{"name":".hidden","source":"project","content":"1;"}`,
		`{"name":"","source":"project","content":"1;"}`,
		`{"name":"ok","source":"tinkerwell","content":"1;"}`,
		`{"name":"ok","source":"","content":"1;"}`,
		`{"name":"ok","source":"project","content":"  "}`,
		// Over the 64 KB cap the listing would never show the file again.
		`{"name":"ok","source":"project","content":"` + strings.Repeat("a", 65*1024) + `"}`,
	}
	for _, body := range bad {
		if rec := postSnippet(t, "acme.test", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status %d, want 400", body, rec.Code)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(sitePath, ".lerd")); err == nil {
		t.Errorf("no file may be written on rejection, found %v", entries)
	}
}

// DELETE removes the named snippet from its source dir and answers the
// refreshed list; the tinkerwell dir is never touched.
func TestDeleteTinkerSnippet_http(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	dir := filepath.Join(sitePath, ".lerd", "tinker", "snippets")
	writeSnippet(t, dir, "gone.php", "1;")
	writeSnippet(t, filepath.Join(sitePath, ".tinkerwell", "snippets"), "keep.php", "2;")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	del := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/acme.test/tinker:snippets?"+query, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		handleSiteAction(rec, req)
		return rec
	}

	rec := del("source=project&name=gone.php")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.php")); !os.IsNotExist(err) {
		t.Fatal("file still exists after delete")
	}
	var list []tinkerSnippet
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "keep.php" {
		t.Fatalf("refreshed list: %+v", list)
	}

	if rec := del("source=project&name=gone.php"); rec.Code != http.StatusNotFound {
		t.Errorf("missing file: status %d, want 404", rec.Code)
	}
	if rec := del("source=tinkerwell&name=keep.php"); rec.Code != http.StatusBadRequest {
		t.Errorf("tinkerwell delete: status %d, want 400", rec.Code)
	}
	if rec := del("source=project&name=../../evil.php"); rec.Code != http.StatusBadRequest {
		t.Errorf("traversal delete: status %d, want 400", rec.Code)
	}
}
