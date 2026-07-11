package ui

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// handleSiteAction routes GET /api/sites/{domain}/env to handleSiteEnv and
// returns the raw .env contents verbatim, preserving comments and ordering
// so the UI can show the file as-is.
func TestHandleSiteEnv_returnsRawContents(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	envBody := "# header comment\nDB_HOST=127.0.0.1\nDB_PORT=3306\n\nMAIL_HOST=mailhog\n"
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte(envBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != envBody {
		t.Errorf("body mismatch\n got: %q\nwant: %q", got, envBody)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type: got %q want text/plain; charset=utf-8", ct)
	}
}

// /env/propose returns a merge that inserts the example keys the .env lacks,
// each placed beside its neighbours, with the required/optional split so the UI
// can offer to also pull in the optional keys.
func TestHandleSiteEnvPropose_insertsInPlace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env.example"),
		[]byte("DB_HOST=localhost\nDB_PORT=5432\nDB_DATABASE=app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env"),
		[]byte("DB_HOST=lerd-postgres\nDB_DATABASE=app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "prop", Path: sitePath, Domains: []string{"prop.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/prop.test/env/propose", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvProposeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Added) != 1 || resp.Added[0] != "DB_PORT" {
		t.Errorf("Added = %v, want [DB_PORT]", resp.Added)
	}
	want := "DB_HOST=lerd-postgres\nDB_PORT=5432\nDB_DATABASE=app\n"
	if resp.Merged != want {
		t.Errorf("Merged mismatch\n got: %q\nwant: %q", resp.Merged, want)
	}
	// DB_PORT is line 2 in the merged output, and it's the one inserted line.
	if len(resp.AddedLines) != 1 || resp.AddedLines[0] != 2 {
		t.Errorf("AddedLines = %v, want [2]", resp.AddedLines)
	}
	if resp.File != ".env" {
		t.Errorf("File = %q, want .env", resp.File)
	}
	// Every missing key is listed with the value that would be written, so the
	// UI can show the user what each key resolves to before adding it.
	if len(resp.Entries) != 1 || resp.Entries[0].Key != "DB_PORT" || resp.Entries[0].Value != "5432" {
		t.Errorf("Entries = %+v, want one DB_PORT=5432", resp.Entries)
	}
}

// /env/propose?keys=A,B stages only the picked keys, and ignores names that
// aren't among the missing example keys.
func TestHandleSiteEnvPropose_selectsKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env.example"),
		[]byte("DB_HOST=localhost\nDB_PORT=5432\nMAIL_HOST=smtp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env"),
		[]byte("DB_HOST=lerd-postgres\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "sel", Path: sitePath, Domains: []string{"sel.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/sel.test/env/propose?keys=DB_PORT,NOPE", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvProposeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Added) != 1 || resp.Added[0] != "DB_PORT" {
		t.Errorf("Added = %v, want [DB_PORT]", resp.Added)
	}
	want := "DB_HOST=lerd-postgres\nDB_PORT=5432\n"
	if resp.Merged != want {
		t.Errorf("Merged mismatch\n got: %q\nwant: %q", resp.Merged, want)
	}
}

// Missing .env returns 200 with an empty body so the UI's gate falls back
// gracefully instead of producing a noisy 404 in the network panel.
func TestHandleSiteEnv_missingFileReturnsEmptyBody(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := config.AddSite(config.Site{Name: "noenv", Path: t.TempDir(), Domains: []string{"noenv.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/noenv.test/env", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", rec.Body.String())
	}
}

// Only GET (read) and PUT (write) are valid on /env. POST and friends stay
// 405 so a future shared dispatcher cannot quietly widen the contract.
func TestHandleSiteEnv_postStillRejected(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/env", nil)
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSiteEnv_putWritesNewFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(SiteEnvWriteRequest{Content: "APP_KEY=base64:abc\nDB_HOST=127.0.0.1\n", Backup: false})
	req := httptest.NewRequest(http.MethodPut, "/api/sites/acme.test/env", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvWriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if resp.BackupPath != "" {
		t.Errorf("BackupPath: got %q want \"\" when backup=false", resp.BackupPath)
	}
	got, err := os.ReadFile(filepath.Join(sitePath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "APP_KEY=base64:abc\nDB_HOST=127.0.0.1\n" {
		t.Errorf("file body mismatch: got %q", string(got))
	}
}

func TestHandleSiteEnv_putPreservesMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	envPath := filepath.Join(sitePath, ".env")
	if err := os.WriteFile(envPath, []byte("OLD=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(SiteEnvWriteRequest{Content: "NEW=2\n", Backup: false})
	req := httptest.NewRequest(http.MethodPut, "/api/sites/acme.test/env", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fs.FileMode(0o600) {
		t.Errorf("mode: got %o want 0600", info.Mode().Perm())
	}
}

func TestHandleSiteEnv_putCreatesBackup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	envPath := filepath.Join(sitePath, ".env")
	oldBody := "DB_PASSWORD=hunter2\n"
	if err := os.WriteFile(envPath, []byte(oldBody), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(SiteEnvWriteRequest{Content: "DB_PASSWORD=correcthorse\n", Backup: true})
	req := httptest.NewRequest(http.MethodPut, "/api/sites/acme.test/env", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvWriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if resp.BackupPath == "" {
		t.Fatal("expected BackupPath set when backup=true")
	}
	if !strings.HasPrefix(resp.BackupPath, ".env.bkp.") {
		t.Errorf("BackupPath %q does not start with .env.bkp.", resp.BackupPath)
	}
	bak, err := os.ReadFile(filepath.Join(sitePath, resp.BackupPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != oldBody {
		t.Errorf("backup body mismatch: got %q want %q", string(bak), oldBody)
	}
	info, err := os.Stat(filepath.Join(sitePath, resp.BackupPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fs.FileMode(0o640) {
		t.Errorf("backup mode: got %o want 0640", info.Mode().Perm())
	}
}

// withCORS must advertise PUT alongside GET and POST so that browser
// preflights for the env-write endpoint do not strip the actual request.
// Regression guard for "Failed to fetch" on Save in the dashboard.
func TestWithCORS_advertisesPUT(t *testing.T) {
	handler := withCORS(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/sites/acme.test/env", nil)
	req.Header.Set("Origin", "http://lerd.localhost")
	rec := httptest.NewRecorder()
	handler(rec, req)

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, "PUT") {
		t.Errorf("Allow-Methods does not include PUT: %q", methods)
	}
}

func TestListEnvFiles_returnsEnvVariantsWithDefaultFirst(t *testing.T) {
	dir := t.TempDir()
	must := func(name string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	must(".env", 0o644)
	must(".env.testing", 0o644)
	must(".env.local", 0o644)
	must(".env.example", 0o644)
	must(".env.bkp.20260528-103045", 0o644)         // backup of .env, excluded
	must(".env.testing.bkp.20260528-103045", 0o644) // backup of .env.testing, excluded
	must(".env.before_lerd", 0o644)                 // lerd's own restore file, excluded
	must(".env.tmp.abc", 0o644)                     // matches via two-segment, excluded by regex
	must("regular.txt", 0o644)                      // not an env file

	got, err := listEnvFiles("", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".env", ".env.example", ".env.local", ".env.testing"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestEnvFileFromQuery(t *testing.T) {
	cases := []struct {
		q        string
		wantFile string
		wantOK   bool
	}{
		{"", ".env", true},
		{"file=.env", ".env", true},
		{"file=.env.testing", ".env.testing", true},
		{"file=.env.local", ".env.local", true},
		{"file=.env.before_lerd", "", false},
		{"file=.env.bkp.20260528-103045", "", false}, // backup, two-segment suffix
		{"file=../etc/passwd", "", false},
		{"file=.env/extra", "", false},
		{"file=other.txt", "", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/?"+c.q, nil)
		gotFile, gotOK := envFileFromQuery(req, ".env")
		if gotOK != c.wantOK {
			t.Errorf("q=%q ok: got %v want %v", c.q, gotOK, c.wantOK)
		}
		if gotOK && gotFile != c.wantFile {
			t.Errorf("q=%q file: got %q want %q", c.q, gotFile, c.wantFile)
		}
	}
}

func TestHandleSiteEnv_filesListAndPerFileReadWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("APP=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.testing"), []byte("TEST=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env/files", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", rec.Code, rec.Body.String())
	}
	var files []string
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != ".env" || files[1] != ".env.testing" {
		t.Errorf("file list: got %v", files)
	}

	// Read with file=.env.testing
	req = httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env?file=.env.testing", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read status %d", rec.Code)
	}
	if got := rec.Body.String(); got != "TEST=1\n" {
		t.Errorf("read body: got %q want %q", got, "TEST=1\n")
	}

	// Write to .env.testing with backup
	body, _ := json.Marshal(SiteEnvWriteRequest{Content: "TEST=2\n", Backup: true})
	req = httptest.NewRequest(http.MethodPut, "/api/sites/acme.test/env?file=.env.testing", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("write status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvWriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if !strings.HasPrefix(resp.BackupPath, ".env.testing.bkp.") {
		t.Errorf("BackupPath: got %q want prefix .env.testing.bkp.", resp.BackupPath)
	}
	// Original .env must not have been touched.
	got, err := os.ReadFile(filepath.Join(sitePath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "APP=1\n" {
		t.Errorf(".env contaminated: %q", string(got))
	}
}

func TestHandleSiteEnv_restoreIsScopedToFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("NEW=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.testing"), []byte("TNEW=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Two backups: one of .env, one of .env.testing.
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260528-103045"), []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.testing.bkp.20260528-103045"), []byte("TOLD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/env/restore?file=.env.testing", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvRestoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if resp.Restored != ".env.testing.bkp.20260528-103045" {
		t.Errorf("Restored: got %q", resp.Restored)
	}
	// .env.testing reverted, .env intact, .env backup untouched.
	got, _ := os.ReadFile(filepath.Join(sitePath, ".env.testing"))
	if string(got) != "TOLD=1\n" {
		t.Errorf(".env.testing: got %q want TOLD=1", string(got))
	}
	got, _ = os.ReadFile(filepath.Join(sitePath, ".env"))
	if string(got) != "NEW=2\n" {
		t.Errorf(".env touched: got %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(sitePath, ".env.bkp.20260528-103045")); err != nil {
		t.Errorf(".env backup gone: %v", err)
	}
}

func TestHandleSiteEnv_restoreUsesMostRecentBackup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("NEW=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260101-100000"), []byte("ANCIENT=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260528-103045"), []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/env/restore", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvRestoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if resp.Restored != ".env.bkp.20260528-103045" {
		t.Errorf("Restored: got %q want %q", resp.Restored, ".env.bkp.20260528-103045")
	}
	if resp.Content != "OLD=1\n" {
		t.Errorf("Content: got %q want %q", resp.Content, "OLD=1\n")
	}
	// Ancient backup should still be on disk; only the restored one is removed.
	if _, err := os.Stat(filepath.Join(sitePath, ".env.bkp.20260101-100000")); err != nil {
		t.Errorf("ancient backup gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sitePath, ".env.bkp.20260528-103045")); !os.IsNotExist(err) {
		t.Errorf("restored backup not removed: err=%v", err)
	}
}

func TestHandleSiteEnv_restoreHonoursNamedBackup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("NEW=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260101-100000"), []byte("ANCIENT=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260528-103045"), []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	// Ask for the OLDER backup by name; it must win over the newest.
	body := strings.NewReader(`{"name":".env.bkp.20260101-100000"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/env/restore", body)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp SiteEnvRestoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("ok=false: %q", resp.Error)
	}
	if resp.Restored != ".env.bkp.20260101-100000" {
		t.Errorf("Restored: got %q want the named older backup", resp.Restored)
	}
	if resp.Content != "ANCIENT=0\n" {
		t.Errorf("Content: got %q want ANCIENT=0", resp.Content)
	}
	got, _ := os.ReadFile(filepath.Join(sitePath, ".env"))
	if string(got) != "ANCIENT=0\n" {
		t.Errorf(".env: got %q want ANCIENT=0", string(got))
	}
	// The newer backup the user did NOT pick must stay on disk.
	if _, err := os.Stat(filepath.Join(sitePath, ".env.bkp.20260528-103045")); err != nil {
		t.Errorf("unpicked newer backup gone: %v", err)
	}
}

func TestHandleSiteEnv_restoreWithoutBackupReturnsError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/env/restore", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp SiteEnvRestoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Error("expected ok=false when no backup exists")
	}
	if !strings.Contains(resp.Error, "no backup") {
		t.Errorf("error: got %q want substring 'no backup'", resp.Error)
	}
}

func TestHandleSiteEnv_backupContentServesRawBytes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	body := "OLD=1\n"
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260528-103045"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env/backups/.env.bkp.20260528-103045", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != body {
		t.Errorf("body: got %q want %q", got, body)
	}
}

func TestHandleSiteEnv_backupContentRejectsTraversalAndOtherNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env.before_lerd"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	// Path traversal: the / segment makes parts longer than 4, falling
	// through to the no-match branch.
	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env/backups/.env.before_lerd", nil)
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("non-backup name: got %d want 404", rec.Code)
	}
}

func TestHandleSiteEnv_backupsListsNewestFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260101-100000"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".env.bkp.20260528-103045"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/env/backups", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var list []SiteEnvBackup
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len: got %d want 2", len(list))
	}
	if list[0].Name != ".env.bkp.20260528-103045" {
		t.Errorf("newest: got %q want %q", list[0].Name, ".env.bkp.20260528-103045")
	}
}

// siteHasEnv distinguishes "file present" from "directory present" so the
// UI only surfaces the Env tab for sites whose root has a real .env file.
func TestSiteHasEnv(t *testing.T) {
	dir := t.TempDir()
	if siteHasEnv("", dir) {
		t.Error("expected false when .env missing")
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !siteHasEnv("", dir) {
		t.Error("expected true after writing .env")
	}

	// A directory named .env (legal on disk) must not count as a usable env file.
	dirOnly := t.TempDir()
	if err := os.Mkdir(filepath.Join(dirOnly, ".env"), 0o755); err != nil {
		t.Fatal(err)
	}
	if siteHasEnv("", dirOnly) {
		t.Error("expected false when .env is a directory")
	}
}

// A framework whose dotenv lives in a subdirectory (CakePHP config/.env) must
// surface the Env tab, list that file, and read it through ?file=, even though
// the file name contains a slash the root-only regex rejects.
func TestSiteEnv_frameworkSubdirDotenv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Register a custom framework pointing its env at config/.env.
	fwDir := config.FrameworksDir()
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fwDir, "cakelike.yaml"),
		[]byte("name: cakelike\nlabel: CakeLike\nenv:\n  file: config/.env\n  format: dotenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sitePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sitePath, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, "config", ".env"), []byte("DEBUG=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !siteHasEnv("cakelike", sitePath) {
		t.Error("expected has_env true for config/.env")
	}

	files, err := listEnvFiles("cakelike", sitePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "config/.env" {
		t.Errorf("listEnvFiles: got %v want [config/.env]", files)
	}

	// The declared subdir file is allowed through the query even with its slash.
	req := httptest.NewRequest(http.MethodGet, "/?file=config/.env", nil)
	if got, ok := envFileFromQuery(req, "config/.env"); !ok || got != "config/.env" {
		t.Errorf("envFileFromQuery(config/.env): got %q ok=%v", got, ok)
	}
	// An unrelated slashed name is still rejected.
	req = httptest.NewRequest(http.MethodGet, "/?file=config/other", nil)
	if _, ok := envFileFromQuery(req, "config/.env"); ok {
		t.Error("expected config/other to be rejected")
	}
}

// A non-dotenv framework (env stored in PHP source) gets no Env tab: siteHasEnv
// is false even when a stray root .env exists.
func TestSiteHasEnv_nonDotenvFrameworkExcluded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	fwDir := config.FrameworksDir()
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fwDir, "phpenv.yaml"),
		[]byte("name: phpenv\nlabel: PHPEnv\nenv:\n  file: wp-config.php\n  format: php-const\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if siteHasEnv("phpenv", sitePath) {
		t.Error("expected false: php-const framework must not surface the Env tab")
	}
}

// laravelAppName surfaces APP_NAME from .env, but only for Laravel projects so
// the sites dashboard can title a tile by its friendly name. Non-Laravel sites,
// a missing .env, or an absent APP_NAME all yield "" and fall back to the domain.
func TestLaravelAppName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_NAME=\"My Shop\"\nAPP_URL=http://shop.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := laravelAppName("laravel", dir); got != "My Shop" {
		t.Errorf("laravel APP_NAME: got %q want %q", got, "My Shop")
	}
	if got := laravelAppName("nextjs", dir); got != "" {
		t.Errorf("non-laravel framework: got %q want empty", got)
	}
	if got := laravelAppName("laravel", t.TempDir()); got != "" {
		t.Errorf("missing .env: got %q want empty", got)
	}
	if got := laravelAppName("laravel", ""); got != "" {
		t.Errorf("empty path: got %q want empty", got)
	}

	noName := t.TempDir()
	if err := os.WriteFile(filepath.Join(noName, ".env"), []byte("APP_URL=http://x.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := laravelAppName("laravel", noName); got != "" {
		t.Errorf("APP_NAME absent: got %q want empty", got)
	}

	// The stock APP_NAME=Laravel default is treated as uncustomised (any case)
	// so the tile falls back to the domain instead of a generic "Laravel" label.
	for _, val := range []string{"Laravel", "laravel", "LARAVEL"} {
		stock := t.TempDir()
		if err := os.WriteFile(filepath.Join(stock, ".env"), []byte("APP_NAME="+val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := laravelAppName("laravel", stock); got != "" {
			t.Errorf("default APP_NAME %q: got %q want empty", val, got)
		}
	}
}
