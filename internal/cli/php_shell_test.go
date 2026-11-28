package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

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
	args := phpShellExecArgs("lerd-php86-fpm", "", "s1")
	for i, a := range args {
		if a == "-w" {
			t.Fatalf("args = %v, want no -w for a project-less shell (found at %d)", args, i)
		}
	}
	if args[len(args)-3] != "sh" || args[len(args)-2] != "-c" {
		t.Errorf("args = %v, want the container script last", args)
	}

	withDir := phpShellExecArgs("lerd-php86-fpm", "/srv/app", "s1")
	if !slices.Contains(withDir, "-w") || !slices.Contains(withDir, "/srv/app") {
		t.Errorf("args = %v, want the project directory kept", withDir)
	}
}

// An interactive shell is a one-shot command like any other, so a provider's
// declared variables have to reach it too, forwarded by name.
func TestPhpShellExecArgsForwardsPassthroughEnv(t *testing.T) {
	t.Setenv("LERD_PASSTHROUGH_ENV", "APP_KEY,DB_PASSWORD")
	t.Setenv("APP_KEY", "base64:abc")
	t.Setenv("DB_PASSWORD", "secret")

	args := phpShellExecArgs("lerd-php86-fpm", "/srv/app", "s1")
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

// The shell is marked with its session, which everything it starts inherits,
// so lerd can find it in the container again to hang up on it.
func TestPhpShellExecArgs_MarksTheSession(t *testing.T) {
	args := phpShellExecArgs("lerd-php86-fpm", "/srv/app", "abc123")
	mark := slices.Index(args, "LERD_SHELL_SESSION=abc123")
	if mark < 1 || args[mark-1] != "--env" || mark > slices.Index(args, "lerd-php86-fpm") {
		t.Errorf("args = %v, want --env LERD_SHELL_SESSION=abc123 before the container", args)
	}
}

// podman exec ends without passing a hang-up on, so the shell in the container
// would run on after its terminal closed. The script lerd runs in the container
// then hangs up on that session's processes, and on no others.
func TestShellHangUpScript_HangsUpOnlyItsSession(t *testing.T) {
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skip("needs /proc, as the container has")
	}
	dir := t.TempDir()
	start := func(session, name string) *exec.Cmd {
		// The child stands in for whatever the shell is running, which
		// inherits the mark.
		script := "trap 'echo hup >" + filepath.Join(dir, name) + "; exit 0' HUP; sh -c 'trap \"echo hup >" + filepath.Join(dir, name+"-child") + "; exit 0\" HUP; while :; do sleep 0.05; done' & while :; do sleep 0.05; done"
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "LERD_SHELL_SESSION="+session)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	start("abc123", "mine")
	start("other", "theirs")
	time.Sleep(300 * time.Millisecond)

	if out, err := exec.Command("sh", "-c", shellHangUpScript("abc123")).CombinedOutput(); err != nil {
		t.Fatalf("hang-up script: %v %s", err, out)
	}
	time.Sleep(300 * time.Millisecond)
	for _, f := range []string{"mine", "mine-child"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was not hung up on", f)
		}
	}
	for _, f := range []string{"theirs", "theirs-child"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s belongs to another session but was hung up on", f)
		}
	}
}
