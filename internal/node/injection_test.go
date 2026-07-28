package node

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hostileVersions are the shapes a repository-supplied version could take to
// reach the shell. Each is run through a real shell below rather than pattern
// matched, so the assertion is "nothing executed", not "the string looks safe".
var hostileVersions = []struct{ name, version string }{
	{"semicolon", "22 --using=x; touch MARKER; #"},
	{"single quote", "22'; touch MARKER; echo '"},
	{"command substitution", "22$(touch MARKER)"},
	{"backtick", "22`touch MARKER`"},
	{"pipe", "22 | touch MARKER"},
	{"newline", "22\ntouch MARKER\n"},
}

// runsFragment executes "<prefix> true" through sh the way a generated worker
// unit does, and reports whether the marker file appeared.
func runsFragment(t *testing.T, dir, prefix string) bool {
	t.Helper()
	marker := filepath.Join(dir, "MARKER")
	_ = os.Remove(marker)
	script := strings.ReplaceAll(prefix, "MARKER", marker) + " true"
	// The same wrapper worker_linux.go builds: the fragment is the body of an
	// sh -c, with single quotes escaped for the surrounding systemd line.
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Dir = dir
	_ = cmd.Run() // a failing fnm/nvm is fine; only the marker matters
	_, err := os.Stat(marker)
	return err == nil
}

// A version that reaches ExecPrefix must never be able to run a command of its
// own, whichever manager is driving Node.
func TestExecPrefixDoesNotExecuteAVersion(t *testing.T) {
	dir := t.TempDir()
	for _, mgr := range []Manager{fnmManager{}, nvmManager{}} {
		for _, tc := range hostileVersions {
			t.Run(mgr.Name()+"/"+tc.name, func(t *testing.T) {
				if runsFragment(t, dir, mgr.ExecPrefix(tc.version)) {
					t.Errorf("%s ExecPrefix executed a command from the version %q", mgr.Name(), tc.version)
				}
			})
		}
	}
}

// nvm drives Node through a bash script built in-process, so the same version
// reaches a shell that way too.
func TestNvmCommandDoesNotExecuteAVersion(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range hostileVersions {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(dir, "MARKER")
			_ = os.Remove(marker)
			cmd := nvmManager{}.Command(strings.ReplaceAll(tc.version, "MARKER", marker), "true", nil)
			cmd.Dir = dir
			_ = cmd.Run()
			if _, err := os.Stat(marker); err == nil {
				t.Errorf("nvm Command executed a command from the version %q", tc.version)
			}
		})
	}
}

// The committed project file is the one version source that reaches a shell
// without passing through the numeric check .nvmrc and .node-version get, so a
// value that could not be a version must not come back from detection.
func TestDetectVersionRejectsAnUnsafeProjectPin(t *testing.T) {
	for _, tc := range hostileVersions {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "node_version: " + strconv_Quote(tc.version) + "\n"
			if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			v, err := DetectVersion(dir)
			if err == nil && v == tc.version {
				t.Errorf("DetectVersion returned the unsafe pin %q verbatim", v)
			}
			if SafeVersion(v) == "" && v != "" {
				t.Errorf("DetectVersion returned %q, which is still not shell-safe", v)
			}
		})
	}
}

// A legitimate pin must survive untouched: reducing 20.11.0 to 20 would run a
// different Node than the project asked for.
func TestDetectVersionKeepsALegitimateProjectPin(t *testing.T) {
	for _, want := range []string{"22", "20.11.0", "v18.20.4", "lts/iron", "default"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("node_version: "+strconv_Quote(want)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := DetectVersion(dir)
		if err != nil || got != want {
			t.Errorf("DetectVersion(%q) = %q, %v; want it kept", want, got, err)
		}
	}
}

func strconv_Quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}
