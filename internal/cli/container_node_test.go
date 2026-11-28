package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/geodro/lerd/internal/podman"
)

// siteWithNode makes a site pinned to Node 22 through .nvmrc, with or without
// a musl build of it on disk.
func siteWithNode(t *testing.T, built bool) (site, bin string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	site = t.TempDir()
	if err := os.WriteFile(filepath.Join(site, ".nvmrc"), []byte("22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(nodeDet.MuslRoot(), "v22.23.3", "bin")
	if built {
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return site, bin
}

func TestWithContainerNode_LeadsTheImagePath(t *testing.T) {
	site, bin := siteWithNode(t, true)
	in := "/app/vendor/bin:" + podman.ContainerPath + ":/composer/bin"
	want := "/app/vendor/bin:" + bin + ":" + podman.ContainerPath + ":/composer/bin"
	if got := withContainerNode(site, in); got != want {
		t.Errorf("withContainerNode = %q, want %q", got, want)
	}
}

func TestWithContainerNode_NoBuildLeavesThePath(t *testing.T) {
	site, _ := siteWithNode(t, false)
	if got := withContainerNode(site, podman.ContainerPath); got != podman.ContainerPath {
		t.Errorf("withContainerNode = %q; without a build the image's Node stays", got)
	}
}

// The shell is where the mismatch showed: starship there reported the image's
// Node while the dashboard reported the site's.
func TestPhpShellExecArgs_CarriesTheSitesNode(t *testing.T) {
	site, bin := siteWithNode(t, true)
	args := phpShellExecArgs("lerd-php85-fpm", site, "s1")
	i := slices.Index(args, "PATH="+bin+":"+podman.ContainerPath)
	if i < 1 || args[i-1] != "--env" || i > slices.Index(args, "lerd-php85-fpm") {
		t.Errorf("the shell's PATH must lead with the site's Node: %q", args)
	}
}

func TestContainerExecEnvArgs_CarriesTheSitesNode(t *testing.T) {
	site, bin := siteWithNode(t, true)
	for _, a := range containerExecEnvArgs(site) {
		if strings.HasPrefix(a, "PATH=") && strings.Contains(a, bin+":"+podman.ContainerPath) {
			return
		}
	}
	t.Errorf("php and composer execs must see the site's Node: %q", containerExecEnvArgs(site))
}

// The containers mount the home folder alone, so a build outside it would put
// a path on PATH they cannot see.
func TestContainerNodeBin_IgnoresABuildOutsideHome(t *testing.T) {
	site, _ := siteWithNode(t, true)
	t.Setenv("HOME", t.TempDir())
	if got := containerNodeBin(site); got != "" {
		t.Errorf("containerNodeBin = %q for a build outside home, want none", got)
	}
}
