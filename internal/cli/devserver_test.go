package cli

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func viteDevServer() *config.DevServerTool {
	return &config.DevServerTool{
		Name:          "vite",
		Base:          "/@lerd-vite/",
		ProjectConfig: []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"},
		WrapperPath:   "node_modules/.lerd/vite.config.mjs",
		Args:          "--config {config} --port {port} --strictPort",
		DefaultPort:   5173,
		Wrapper: `import { mergeConfig } from 'vite';
import projectConfig from '%s';
const lerd = { base: '%s', server: { origin: '%s', allowedHosts: %s } };
export default projectConfig;
`,
	}
}

func gitRepo(t *testing.T, ignore string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	if ignore != "" {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDevServerProjectConfigPicksFirstThatExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vite.config.ts"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := devServerProjectConfig(dir, viteDevServer())
	if got != "vite.config.ts" {
		t.Fatalf("devServerProjectConfig() = %q, want vite.config.ts", got)
	}
}

// No config file means the project is not shaped for this, and the dev server
// has to be left exactly as it was.
func TestDevServerProjectConfigMissingIsEmpty(t *testing.T) {
	if got := devServerProjectConfig(t.TempDir(), viteDevServer()); got != "" {
		t.Fatalf("devServerProjectConfig() = %q, want empty", got)
	}
}

func TestWriteDevServerWrapperMergesBaseAndOrigin(t *testing.T) {
	dir := gitRepo(t, "/node_modules\n")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := writeDevServerWrapper(dir, viteDevServer(), "https://myapp.test", []string{"myapp.test"})
	if err != nil {
		t.Fatalf("writeDevServerWrapper() error: %v", err)
	}
	if rel != "node_modules/.lerd/vite.config.mjs" {
		t.Fatalf("wrapper path = %q", rel)
	}

	body, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"'/@lerd-vite/'",
		"'https://myapp.test'",
		`"myapp.test"`,
		"'../../vite.config.js'",
		"mergeConfig",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("wrapper missing %s:\n%s", want, body)
		}
	}
}

// A worktree seeds node_modules from its parent by reflink, so a wrapper left
// over from the parent carries the wrong origin and must be overwritten.
func TestWriteDevServerWrapperOverwritesInheritedCopy(t *testing.T) {
	dir := gitRepo(t, "/node_modules\n")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "node_modules", ".lerd")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "vite.config.mjs"), []byte("origin: 'https://parent.test'"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := writeDevServerWrapper(dir, viteDevServer(), "https://feature.myapp.test", []string{"feature.myapp.test"})
	if err != nil {
		t.Fatalf("writeDevServerWrapper() error: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "parent.test") {
		t.Errorf("inherited wrapper survived:\n%s", body)
	}
	if !strings.Contains(string(body), "feature.myapp.test") {
		t.Errorf("wrapper does not carry this worktree's origin:\n%s", body)
	}
}

// Writing into a path the project tracks would put a generated file in someone's
// commit, so the whole mechanism steps aside instead.
func TestWriteDevServerWrapperSkipsWhenPathIsTracked(t *testing.T) {
	dir := gitRepo(t, "")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := writeDevServerWrapper(dir, viteDevServer(), "https://myapp.test", []string{"myapp.test"})
	if err != nil {
		t.Fatalf("writeDevServerWrapper() error: %v", err)
	}
	if rel != "" {
		t.Fatalf("wrapper path = %q, want empty for a non-ignored path", rel)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules/.lerd/vite.config.mjs")); err == nil {
		t.Error("wrapper was written into a path the project does not ignore")
	}
}

// An ignore rule does not settle it: a project that has actually committed the
// path would get a generated file rewritten under it on every start.
func TestWriteDevServerWrapperSkipsWhenPathIsCommitted(t *testing.T) {
	dir := gitRepo(t, "/node_modules\n")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(dir, "node_modules", ".lerd")
	if err := os.MkdirAll(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrapper, "vite.config.mjs"), []byte("// theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "add", "-f", "node_modules/.lerd/vite.config.mjs")
	add.Dir = dir
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, out)
	}

	rel, err := writeDevServerWrapper(dir, viteDevServer(), "https://myapp.test", []string{"myapp.test"})
	if err != nil {
		t.Fatalf("writeDevServerWrapper() error: %v", err)
	}
	if rel != "" {
		t.Fatalf("wrapper path = %q, want empty for a committed path", rel)
	}
	body, err := os.ReadFile(filepath.Join(wrapper, "vite.config.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "// theirs" {
		t.Errorf("a committed file was overwritten: %s", body)
	}
}

func TestDevServerArgsSubstitutesConfigAndPort(t *testing.T) {
	got := devServerArgs(viteDevServer(), "node_modules/.lerd/vite.config.mjs", 5180)
	want := "--config node_modules/.lerd/vite.config.mjs --port 5180 --strictPort"
	if got != want {
		t.Fatalf("devServerArgs() = %q, want %q", got, want)
	}
}

// Without a wrapper there is nothing to point the tool at, so the command keeps
// its own defaults rather than being handed a half-applied flag set.
func TestDevServerArgsEmptyWithoutWrapper(t *testing.T) {
	if got := devServerArgs(viteDevServer(), "", 5180); got != "" {
		t.Fatalf("devServerArgs() = %q, want empty", got)
	}
}

// A project that is not under git at all has no commit for a generated file to
// land in, so the mechanism applies rather than standing down.
func TestWriteDevServerWrapperAppliesOutsideGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := writeDevServerWrapper(dir, viteDevServer(), "https://myapp.test", []string{"myapp.test"})
	if err != nil {
		t.Fatalf("writeDevServerWrapper() error: %v", err)
	}
	if rel == "" {
		t.Fatal("wrapper skipped for a project outside git")
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
		t.Errorf("wrapper not written: %v", err)
	}
}

// A port nothing has claimed in the registry can still be held by a dev server
// that predates pinning. Handing it out makes the tool refuse to start, since
// it is launched with strict-port rather than left to drift.
func TestAssignDevServerPortSkipsPortsInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	got := assignDevServerPort("myapp", "", busy)
	if got == busy {
		t.Fatalf("assignDevServerPort() handed out port %d, which is already bound", busy)
	}
	if got < busy {
		t.Fatalf("assignDevServerPort() = %d, want a port at or above %d", got, busy)
	}
}

// A worktree runs its own dev server, so it must never be handed the port its
// parent site is already pinned to.
func TestAssignDevServerPortKeepsWorktreesOffTheParentPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := config.AddSite(config.Site{
		Name:             "myapp",
		Path:             filepath.Join(dir, "myapp"),
		Domains:          []string{"myapp.test"},
		DevServerPort:    5173,
		WorktreeDevPorts: map[string]int{"myapp-feature": 5174},
	}); err != nil {
		t.Fatal(err)
	}

	got := assignDevServerPort("myapp", "myapp-other", 5173)
	if got == 5173 || got == 5174 {
		t.Fatalf("assignDevServerPort() = %d, want a port clear of the site (5173) and its other worktree (5174)", got)
	}
}

// Re-pinning the same worktree has to be able to keep the port it already owns,
// otherwise every restart walks it up by one.
func TestAssignDevServerPortLetsAWorktreeKeepItsOwnPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := config.AddSite(config.Site{
		Name:             "myapp",
		Path:             filepath.Join(dir, "myapp"),
		Domains:          []string{"myapp.test"},
		WorktreeDevPorts: map[string]int{"myapp-feature": 5199},
	}); err != nil {
		t.Fatal(err)
	}

	if got := assignDevServerPort("myapp", "myapp-feature", 5199); got != 5199 {
		t.Fatalf("assignDevServerPort() = %d, want it to keep 5199", got)
	}
}
