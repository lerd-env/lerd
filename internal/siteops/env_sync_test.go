package siteops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A worktree's URL is <branch>.<primary>, so a primary domain change has to
// reach it, Reverb's browser host included, and not only the parent.
func TestSyncEnvIfPrimaryChanged_movesWorktrees(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	main := t.TempDir()
	checkout := t.TempDir()

	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat-x\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)
	os.WriteFile(filepath.Join(main, ".env"), []byte("APP_URL=http://acme.test\nVITE_REVERB_HOST=acme.test\n"), 0644)
	os.WriteFile(filepath.Join(checkout, ".env"), []byte("APP_URL=http://feat-x.acme.test\nVITE_REVERB_HOST=feat-x.acme.test\n"), 0644)

	site := &config.Site{Name: "acme", Path: main, Domains: []string{"shop.test"}}
	if err := SyncEnvIfPrimaryChanged(site, "acme.test"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(checkout, ".env"))
	for _, want := range []string{"APP_URL=http://feat-x.shop.test", "VITE_REVERB_HOST=feat-x.shop.test"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("worktree .env missing %s:\n%s", want, got)
		}
	}
}
