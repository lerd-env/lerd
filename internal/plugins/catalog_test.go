package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRepo makes a repo holding the boilerplate plugin named name and returns
// its path and the commit. A second commit then changes the plugin, so a test
// can tell the approved commit from what the repo has since moved on to.
func gitRepo(t *testing.T, name string) (dir, approved string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "repo")
	if err := Scaffold(name, dir); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "approved")
	approved = git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "LATER"), []byte("not approved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "later")
	return dir, approved
}

// serveIndex points the catalog at a test server answering with listings.
func serveIndex(t *testing.T, listings ...Listing) *int {
	t.Helper()
	hits := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hits++
		_ = json.NewEncoder(w).Encode(Catalog{Plugins: listings})
	}))
	t.Cleanup(srv.Close)
	orig := CatalogURL
	CatalogURL = srv.URL
	t.Cleanup(func() { CatalogURL = orig })
	return hits
}

func TestInstallListed_ChecksOutTheApprovedCommitOnly(t *testing.T) {
	isolate(t)
	repo, approved := gitRepo(t, "hello")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: approved})

	p, err := InstallListed("hello")
	if err != nil {
		t.Fatalf("InstallListed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "LATER")); !os.IsNotExist(err) {
		t.Error("the install must be the approved commit, not the repo's newer head")
	}
	if _, err := os.Stat(filepath.Join(p.Dir, ".git")); !os.IsNotExist(err) {
		t.Error("an installed snapshot keeps no git history to drift with")
	}
	src, ok := InstalledSource("hello")
	if !ok || src.Commit != approved {
		t.Errorf("source = %+v, want the approved commit recorded", src)
	}
}

func TestInstallListed_Refusals(t *testing.T) {
	isolate(t)
	repo, approved := gitRepo(t, "hello")
	serveIndex(t,
		Listing{Name: "mismatch", Repo: "file://" + repo, Commit: approved},
		Listing{Name: "ghost", Repo: "file://" + repo, Commit: strings.Repeat("0", 40)},
	)
	cases := map[string]string{
		"unlisted": "not in the plugin catalog",
		"mismatch": `"hello"`,
		"ghost":    "0000000",
	}
	for name, want := range cases {
		if _, err := InstallListed(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %v, want it to mention %q", name, err, want)
		}
	}
	entries, _ := os.ReadDir(Dir())
	for _, e := range entries {
		t.Errorf("a refused install left %s behind", e.Name())
	}
}

// An update replaces the plugin with the newly approved commit and keeps what
// it stored.
func TestUpdateListed_MovesToTheNewApprovedCommit(t *testing.T) {
	isolate(t)
	repo, first := gitRepo(t, "hello")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: first})
	if _, err := InstallListed("hello"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(DataDirFor("hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	head, _ := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	second := strings.TrimSpace(string(head))
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: second})
	if err := refreshCatalog(); err != nil {
		t.Fatal(err)
	}

	if err := UpdateListed("hello"); err != nil {
		t.Fatalf("UpdateListed: %v", err)
	}
	if src, _ := InstalledSource("hello"); src.Commit != second {
		t.Errorf("installed commit = %s, want %s", src.Commit, second)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "hello", "LATER")); err != nil {
		t.Error("the update should bring the newly approved files")
	}
	if _, err := os.Stat(DataDirFor("hello")); err != nil {
		t.Error("an update must keep the plugin's data")
	}
}

func TestPendingUpdate(t *testing.T) {
	isolate(t)
	repo, approved := gitRepo(t, "hello")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: approved})
	if _, err := InstallListed("hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := PendingUpdate("hello"); ok {
		t.Error("a plugin at its approved commit has no update")
	}
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: strings.Repeat("b", 40)})
	if err := refreshCatalog(); err != nil {
		t.Fatal(err)
	}
	if l, ok := PendingUpdate("hello"); !ok || l.Commit != strings.Repeat("b", 40) {
		t.Errorf("a newer approved commit is an update, got %v %v", l, ok)
	}
	writePlugin(t, Dir(), "local", "api_version: 1\nname: local\n")
	if _, ok := PendingUpdate("local"); ok {
		t.Error("a plugin installed from a folder is never updated from the catalog")
	}
}

func TestIsCatalogName(t *testing.T) {
	dir := t.TempDir()
	for arg, want := range map[string]bool{"hello": true, dir: false, "https://github.com/x/y": false, "git@github.com:x/y.git": false} {
		if got := IsCatalogName(arg); got != want {
			t.Errorf("IsCatalogName(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestCatalog_CachedForADayAndRefusesBadListings(t *testing.T) {
	isolate(t)
	good := Listing{Name: "hello", Repo: "https://github.com/x/hello", Commit: strings.Repeat("a", 40), Images: []string{"images/hello/a.webp"}}
	bad := func(name string, images ...string) Listing {
		return Listing{Name: name, Repo: "https://github.com/x/y", Commit: strings.Repeat("a", 40), Images: images}
	}
	hits := serveIndex(t, good,
		bad("Bad Name"),
		Listing{Name: "short", Repo: "https://github.com/x/y", Commit: "abc"},
		bad("script", "javascript:alert(1)"),
		bad("elsewhere", "https://evil.test/x.png"),
		bad("protocol-relative", "//evil.test/x.png"),
		bad("climbs", "images/../../x.png"),
	)

	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Plugins) != 1 || c.Plugins[0].Name != "hello" {
		t.Fatalf("want only the valid listing, got %+v", c.Plugins)
	}
	if want := strings.TrimSuffix(CatalogURL, "/") + "/images/hello/a.webp"; c.Plugins[0].Images[0] != want {
		t.Errorf("image = %q, want it resolved beside the index as %q", c.Plugins[0].Images[0], want)
	}
	if _, err := LoadCatalog(); err != nil || *hits != 1 {
		t.Errorf("a fresh cache must be reused, fetched %d times", *hits)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(catalogCachePath(), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(); err != nil || *hits != 2 {
		t.Errorf("a day-old cache must be refreshed, fetched %d times", *hits)
	}
}
