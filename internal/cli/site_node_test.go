package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/linker"
)

// stubNode fakes a managed Node with the given majors and returns what gets installed.
func stubNode(t *testing.T, installErr error, installed ...string) *[]string {
	t.Helper()
	prevManages, prevMissing, prevInstall := managesNodeFn, nodeMissingFn, installNodeFn
	t.Cleanup(func() { managesNodeFn, nodeMissingFn, installNodeFn = prevManages, prevMissing, prevInstall })
	var asked []string
	managesNodeFn = func() bool { return true }
	nodeMissingFn = func(v string) bool { return !strings.Contains(" "+strings.Join(installed, " ")+" ", " "+v+" ") }
	installNodeFn = func(v string) error { asked = append(asked, v); return installErr }
	return &asked
}

func writeNodeFixture(t *testing.T, files map[string]string) {
	t.Helper()
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// addNodeWorktree registers a fake git worktree of site and returns its path.
func addNodeWorktree(t *testing.T, site string) string {
	t.Helper()
	wt := t.TempDir()
	meta := filepath.Join(site, ".git", "worktrees", "feat")
	writeNodeFixture(t, map[string]string{
		filepath.Join(meta, "HEAD"):   "ref: refs/heads/feat\n",
		filepath.Join(meta, "gitdir"): filepath.Join(wt, ".git") + "\n",
		filepath.Join(wt, ".git"):     "gitdir: " + meta + "\n",
	})
	return wt
}

func TestEnsureSiteNode(t *testing.T) {
	for name, tc := range map[string]struct {
		files     map[string]string
		worktree  map[string]string
		container bool
		unmanaged bool
		bun       bool
		want      string
	}{
		"missing":              {files: map[string]string{".nvmrc": "20"}, want: "20"},
		"installed":            {files: map[string]string{".nvmrc": "22"}},
		"not a node project":   {files: map[string]string{".lerd.yaml": "node_version: \"20\"\n"}},
		"node pinned over bun": {files: map[string]string{".nvmrc": "20", ".lerd.yaml": "js_runtime: node\n", "bun.lock": ""}, want: "20"},
		"bun project":          {files: map[string]string{".nvmrc": "20", "bun.lock": ""}, bun: true},
		"custom container":     {files: map[string]string{".nvmrc": "20"}, container: true},
		"user's own node":      {files: map[string]string{".nvmrc": "20"}, unmanaged: true},
		"worktree":             {files: map[string]string{".nvmrc": "22"}, worktree: map[string]string{".nvmrc": "18"}, want: "18"},
		"worktree, same major": {files: map[string]string{".nvmrc": "20"}, worktree: map[string]string{".nvmrc": "20"}, want: "20"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			asked := stubNode(t, nil, "22")
			if tc.unmanaged {
				managesNodeFn = func() bool { return false }
			}
			if tc.bun {
				bin := t.TempDir()
				if err := os.WriteFile(filepath.Join(bin, "bun"), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			}
			site := config.Site{Name: "shop", Path: t.TempDir(), Domains: []string{"shop.test"}}
			if tc.container {
				site.ContainerPort = 3000
			}
			for f, body := range tc.files {
				writeNodeFixture(t, map[string]string{filepath.Join(site.Path, f): body})
			}
			if tc.worktree != nil {
				wt := addNodeWorktree(t, site.Path)
				for f, body := range tc.worktree {
					writeNodeFixture(t, map[string]string{filepath.Join(wt, f): body})
				}
			}
			if err := ensureSiteNode(site, linker.NopReporter{}); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(*asked, ","); got != tc.want {
				t.Errorf("installed %q, want %q", got, tc.want)
			}
		})
	}
}

// Node serves a host-proxy site, so without it the resume stops; a PHP site comes back.
func TestUnpauseSite_nodeInstallFails(t *testing.T) {
	// The real refresh re-executes the test binary.
	prev := hostProxyEnvRefresh
	hostProxyEnvRefresh = func(string) error { return nil }
	t.Cleanup(func() { hostProxyEnvRefresh = prev })
	for _, hostProxy := range []bool{false, true} {
		pausedSite(t)
		stubResumePHP(t, nil, true)
		asked := stubNode(t, errors.New("offline"))
		site, err := config.FindSite("shop")
		if err != nil {
			t.Fatal(err)
		}
		if hostProxy {
			site.HostPort = 5173
			if err := config.AddSite(*site); err != nil {
				t.Fatal(err)
			}
		}
		writeNodeFixture(t, map[string]string{filepath.Join(site.Path, ".nvmrc"): "20"})

		err = UnpauseSite("shop")
		site, _ = config.FindSite("shop")
		if strings.Join(*asked, ",") != "20" || (err != nil) != hostProxy || site.Paused != hostProxy {
			t.Errorf("hostProxy=%v: installed %v, err=%v, paused=%v", hostProxy, *asked, err, site.Paused)
		}
	}
}

func TestLinkDepsInstallNode(t *testing.T) {
	if linkDeps().EnsureNode == nil {
		t.Error("lerd link does not install a missing Node")
	}
}
