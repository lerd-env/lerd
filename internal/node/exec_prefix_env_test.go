package node

import (
	"strings"
	"testing"
)

func TestFnmExecPrefixWithEnv_InjectsAfterActivation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := fnmManager{}
	prefix := m.ExecPrefixWithEnv("default", []string{"npm_config_prefix=/tmp/lerd-global"})
	if !strings.Contains(prefix, "env npm_config_prefix=/tmp/lerd-global") {
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
