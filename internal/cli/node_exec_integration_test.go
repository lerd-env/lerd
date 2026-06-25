package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestRunWithFnm_FailingScriptReturnsErrorNotExit is the end-to-end guard for the
// lerd link/setup regression: a failing `npm run <script>` must return an error
// (so the setup step loop can report it) rather than os.Exit and kill lerd. It
// drives the real fnm/npm toolchain, so it skips when fnm or a default Node isn't
// installed. If exitOnFail were still hard-coded true here, this test process
// would be terminated and the run would fail loudly — which is the point.
func TestRunWithFnm_FailingScriptReturnsErrorNotExit(t *testing.T) {
	fnm := filepath.Join(config.BinDir(), "fnm")
	if _, err := os.Stat(fnm); err != nil {
		t.Skipf("fnm not installed at %s; skipping integration test", fnm)
	}
	// Without a default Node, runWithFnm returns early with the "no Node.js
	// version available" error and never reaches the failing-script branch this
	// test guards, so the skip must check for it too — not just fnm's presence.
	if err := exec.Command(fnm, "exec", "--using=default", "--", "true").Run(); err != nil {
		t.Skip("no default Node available via fnm; skipping integration test")
	}

	dir := t.TempDir()
	pkg := `{"name":"lerd-fail-test","private":true,"scripts":{"production":"node -e \"process.exit(7)\""}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)

	err := runWithFnm("npm", []string{"run", "production"}, false)
	if err == nil {
		t.Fatal("expected an error from a failing npm script, got nil (did the process survive but swallow the failure?)")
	}
}
