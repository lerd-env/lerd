package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposerExecArgsRunsLerdsOwnComposer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	args := composerExecArgs("lerd-php85-fpm", "/home/u/site", []string{"FOO=1"}, []string{"install", "--no-interaction"})

	phar := filepath.Join(tmp, "lerd", "bin", "composer.phar")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "lerd-php85-fpm php "+phar+" install --no-interaction") {
		t.Fatalf("MCP must run lerd's pinned phar, not the image's composer: %v", args)
	}
	if !strings.Contains(joined, "--env COMPOSER_PROCESS_TIMEOUT=") {
		t.Errorf("process timeout not injected: %v", args)
	}
	if !strings.Contains(joined, "--env FOO=1") {
		t.Errorf("caller env not injected: %v", args)
	}
	if !strings.Contains(joined, "-w /home/u/site") {
		t.Errorf("working directory not set: %v", args)
	}
}

func TestComposerExecArgsForwardsProviderEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LERD_PASSTHROUGH_ENV", "VAULT_*")
	t.Setenv("VAULT_TOKEN", "s.secret")

	args := composerExecArgs("lerd-php85-fpm", "/home/u/site", nil, []string{"install"})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--env VAULT_TOKEN ") {
		t.Errorf("provider variable not forwarded by name: %v", args)
	}
	if strings.Contains(joined, "s.secret") {
		t.Errorf("value must stay out of argv: %v", args)
	}
}

func TestComposerExecArgsUsesHostComposerHome(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".composer"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("HOME", home)
	t.Setenv("COMPOSER_HOME", "")
	joined := strings.Join(composerExecArgs("lerd-php85-fpm", "/home/u/site", nil, []string{"install"}), " ")
	if !strings.Contains(joined, "--env HOME="+home+" ") {
		t.Errorf("host HOME not passed: %v", joined)
	}
	if !strings.Contains(joined, "--env COMPOSER_HOME="+filepath.Join(home, ".composer")+" ") {
		t.Errorf("host composer home not passed, a global auth.json would not be read: %v", joined)
	}
}
