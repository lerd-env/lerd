package node

import (
	"os/exec"
	"strings"
	"testing"
)

// The prefix is interpolated verbatim into a /bin/sh wrapper, so the shell is
// the authority on whether a value survives, not a substring match. A path with
// a space splits into two words unless quoted, and the second word becomes the
// command env tries to run, so the wrapper never reaches the real binary.
func TestFnmExecPrefixWithEnv_ValueSurvivesWordSplitting(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	const spaced = "/home/john smith/.local/share/lerd/node-global"
	prefix := fnmManager{}.ExecPrefixWithEnv("default", []string{"npm_config_prefix=" + spaced})

	i := strings.Index(prefix, "env ")
	if i < 0 {
		t.Fatalf("no env injection:\n%s", prefix)
	}
	script := "for w in " + prefix[i+len("env "):] + "; do printf '%s\\n' \"$w\"; done"
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	got := strings.SplitN(strings.TrimRight(string(out), "\n"), "\n", 2)[0]
	if want := "npm_config_prefix=" + spaced; got != want {
		t.Errorf("value did not survive shell splitting:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFnmExecPrefixWithEnv_InjectsAfterActivation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fnmManager{}
	prefix := m.ExecPrefixWithEnv("default", []string{"npm_config_prefix=/tmp/lerd-global"})
	if !strings.Contains(prefix, "env npm_config_prefix="+shellQuote("/tmp/lerd-global")) {
		t.Errorf("fnm ExecPrefixWithEnv missing env injection:\n%s", prefix)
	}
	// env must come after -- so fnm has already activated Node.
	dashIdx := strings.Index(prefix, "--")
	envIdx := strings.Index(prefix, "env ")
	if dashIdx < 0 || envIdx < 0 || envIdx < dashIdx {
		t.Errorf("env must appear after -- in fnm ExecPrefixWithEnv:\n%s", prefix)
	}
}

func TestNvmExecPrefixWithEnv_InjectsAfterNvmUse(t *testing.T) {
	m := nvmManager{}
	prefix := m.ExecPrefixWithEnv("default", []string{"npm_config_prefix=/tmp/lerd-global"})
	useIdx := strings.Index(prefix, "nvm use")
	exportIdx := strings.Index(prefix, "export npm_config_prefix=")
	execIdx := strings.Index(prefix, `exec "$@"`)
	if useIdx < 0 || exportIdx < 0 || execIdx < 0 {
		t.Fatalf("nvm ExecPrefixWithEnv missing pieces:\n%s", prefix)
	}
	if !(useIdx < exportIdx && exportIdx < execIdx) {
		t.Errorf("export must sit after nvm use and before exec:\n%s", prefix)
	}
	if !strings.Contains(prefix, "export npm_config_prefix=") || !strings.Contains(prefix, "/tmp/lerd-global") {
		t.Errorf("expected shell-quoted export of npm_config_prefix:\n%s", prefix)
	}
}

func TestExecPrefixWithEnv_NoEnvReturnsPlainPrefix(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fnmManager{}
	got := m.ExecPrefixWithEnv("default", nil)
	want := m.ExecPrefix("default")
	if got != want {
		t.Errorf("ExecPrefixWithEnv with no env should equal ExecPrefix:\ngot:  %s\nwant: %s", got, want)
	}
}
