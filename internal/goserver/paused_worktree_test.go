package goserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func pausedFixture(t *testing.T, secured bool) *Handler {
	t.Helper()
	h, _ := fixture(t)
	site := h.Sites()[0]
	site.Paused, site.Secured = true, secured
	h.Sites = func() []config.Site { return []config.Site{site} }
	return h
}

func writePausedPage(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(config.PausedDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.PausedDir(), "paused.html"), []byte("<h1>Site Paused</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Every path of a paused site, its subdomains too, answers the paused page.
func TestPaused_everyPathServesThePausedPage(t *testing.T) {
	h := pausedFixture(t, false)
	writePausedPage(t)
	for _, url := range []string{"http://app.test/", "http://app.test/deep/route?x=1", "http://feat.app.test/"} {
		rec := get(h, "GET", url, "")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Site Paused") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: %d %q %q", url, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
	}
}

func TestPaused_securedSiteStillRedirectsToHTTPS(t *testing.T) {
	h := pausedFixture(t, true)
	writePausedPage(t)
	if rec := get(h, "GET", "http://app.test/x", ""); rec.Code != 302 || rec.Header().Get("Location") != "https://app.test/x" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestPaused_missingPageIs503(t *testing.T) {
	h := pausedFixture(t, false)
	if rec := get(h, "GET", "http://app.test/", ""); rec.Code != 503 {
		t.Fatalf("status %d, want 503", rec.Code)
	}
}

// A git worktree of the site is served on <branch>.<domain> from its own
// checkout, as its nginx worktree vhost is.
func TestWorktree_servedFromItsOwnCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	h, _ := fixture(t)
	site := h.Sites()[0]
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(site.Path, "init", "-q", "-b", "main")
	run(site.Path, "add", "-A")
	run(site.Path, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "feat")
	run(site.Path, "worktree", "add", "-q", "-b", "feat", wt)

	body := get(h, "GET", "http://feat.app.test/", "").Body.String()
	want := filepath.Join(wt, "public", "index.php")
	if real, err := filepath.EvalSymlinks(filepath.Join(wt, "public")); err == nil {
		want = filepath.Join(real, "index.php")
	}
	if !strings.HasPrefix(body, want+"|") {
		t.Fatalf("worktree served %q, want script %s", body, want)
	}
	// The site's own domain still serves the main checkout.
	if body := get(h, "GET", "http://app.test/", "").Body.String(); strings.Contains(body, wt) {
		t.Errorf("main domain served the worktree: %q", body)
	}
}
