package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// canonicalTempDir returns a temp dir with symlinks resolved. On macOS /var is a
// symlink to /private/var, and the site registry stores canonical paths, so a
// raw t.TempDir() would not prefix-match the stored site path (the shape
// os.Getwd already returns in production).
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A worktree checked out inside its parent site path-prefix-matches the parent
// in SiteRootFor, so the shell must resolve the worktree checkout itself, or it
// opens the parent tree while running the worktree's own PHP and FPM.
func TestShellWorkDir_NestedWorktreeOpensTheWorktree(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(canonicalTempDir(t), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	if got := shellWorkDir(wt); got != wt {
		t.Errorf("shellWorkDir = %q, want the worktree root %q", got, wt)
	}
}

// From inside a plain registered site the shell opens the site root, wherever in
// the tree the command was run.
func TestShellWorkDir_PlainSiteOpensTheSiteRoot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(canonicalTempDir(t), "app")
	sub := filepath.Join(site, "app", "Http")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	if got := shellWorkDir(sub); got != site {
		t.Errorf("shellWorkDir = %q, want the site root %q", got, site)
	}
}

// A version shell has no project, so pinning the container to a working
// directory would drop it somewhere arbitrary; the image's own default is right.
func TestPhpShellExecArgs_OmitsWorkDirWhenThereIsNoProject(t *testing.T) {
	args := phpShellExecArgs("lerd-php86-fpm", "")
	for i, a := range args {
		if a == "-w" {
			t.Fatalf("args = %v, want no -w for a project-less shell (found at %d)", args, i)
		}
	}
	if args[len(args)-3] != "sh" || args[len(args)-2] != "-c" {
		t.Errorf("args = %v, want the container script last", args)
	}

	withDir := phpShellExecArgs("lerd-php86-fpm", "/srv/app")
	if !slices.Contains(withDir, "-w") || !slices.Contains(withDir, "/srv/app") {
		t.Errorf("args = %v, want the project directory kept", withDir)
	}
}

// The container script prepends to the container's own PATH. It must reach
// podman as one argv element: anything that runs it through a host shell first
// expands $PATH there, and $HOME is bind mounted, so the host's php shim would
// win over the container's own binary.
func TestPhpShellInnerScript_LeavesPathToTheContainer(t *testing.T) {
	if !strings.Contains(phpShellInnerScript(), `"/root/.bun/bin:$PATH"`) {
		t.Errorf("inner script = %q, want the container's own $PATH kept", phpShellInnerScript())
	}
}

// An interactive shell is a one-shot command like any other, so a provider's
// declared variables have to reach it too, forwarded by name.
func TestPhpShellExecArgsForwardsPassthroughEnv(t *testing.T) {
	t.Setenv("LERD_PASSTHROUGH_ENV", "APP_KEY,DB_PASSWORD")
	t.Setenv("APP_KEY", "base64:abc")
	t.Setenv("DB_PASSWORD", "secret")

	args := phpShellExecArgs("lerd-php86-fpm", "/srv/app")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--env APP_KEY --env DB_PASSWORD") {
		t.Errorf("phpShellExecArgs() = %q, missing the forwarded variables", joined)
	}
	for _, a := range args {
		if a == "secret" || strings.Contains(a, "DB_PASSWORD=") {
			t.Errorf("phpShellExecArgs() leaked a value into argv: %q", a)
		}
	}
}
