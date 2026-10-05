package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func sourceRequest(file string, line int) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/source?file=%s&line=%d", url.QueryEscape(file), line), nil)
	r.RemoteAddr = "127.0.0.1:4000"
	w := httptest.NewRecorder()
	handleSource(w, r)
	return w
}

func TestSource_ReadsTheLinesAroundALineInASite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := t.TempDir()
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: site}); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	file := filepath.Join(site, "routes", "web.php")
	os.MkdirAll(filepath.Dir(file), 0o755)                            //nolint:errcheck
	os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644) //nolint:errcheck

	w := sourceRequest(file, 20)
	var got []sourceLine
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if len(got) != 2*sourceContext+1 || got[0].N != 20-sourceContext || got[sourceContext].Text != "line 20" {
		t.Fatalf("lines = %+v", got)
	}
}

// A path the page names outside every site, or one that leaves a site
// through a symlink or .., is never read.
func TestSource_RefusesAFileOutsideTheSites(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site, outside := t.TempDir(), t.TempDir()
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: site}); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret.php")
	os.WriteFile(secret, []byte("<?php\n"), 0o644)      //nolint:errcheck
	os.Symlink(secret, filepath.Join(site, "link.php")) //nolint:errcheck
	for _, file := range []string{secret, filepath.Join(site, "link.php"), site + "/../" + filepath.Base(outside) + "/secret.php", site, "relative.php"} {
		if w := sourceRequest(file, 1); w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", file, w.Code)
		}
	}
}
