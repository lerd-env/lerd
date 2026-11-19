package composer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestProcessTimeoutEnv_DefaultWhenUnset(t *testing.T) {
	if err := os.Unsetenv("COMPOSER_PROCESS_TIMEOUT"); err != nil {
		t.Fatal(err)
	}
	got := ProcessTimeoutEnv()
	want := "COMPOSER_PROCESS_TIMEOUT=" + DefaultProcessTimeout
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProcessTimeoutEnv_HostOverrideWins(t *testing.T) {
	t.Setenv("COMPOSER_PROCESS_TIMEOUT", "0")
	if got := ProcessTimeoutEnv(); got != "COMPOSER_PROCESS_TIMEOUT=0" {
		t.Errorf("host override not honoured: got %q", got)
	}
}

func TestProcessTimeoutEnv_EmptyHostValueFallsBack(t *testing.T) {
	t.Setenv("COMPOSER_PROCESS_TIMEOUT", "")
	got := ProcessTimeoutEnv()
	want := "COMPOSER_PROCESS_TIMEOUT=" + DefaultProcessTimeout
	if got != want {
		t.Errorf("empty host value should fall back to default: got %q", got)
	}
}

func TestPharPathFollowsBinDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	want := filepath.Join(tmp, "lerd", "bin", "composer.phar")
	if got := PharPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Composer scripts (Symfony Flex's auto-scripts) start the container's php on
// their own, so the framework's memory_limit reaches them only through the env.
func TestMemoryLimitEnv(t *testing.T) {
	withLimit := &config.Framework{PHP: config.FrameworkPHP{CLIIni: map[string]string{"memory_limit": "512M"}}}

	// Composer raises itself to 1536M unless COMPOSER_MEMORY_LIMIT is set, so a
	// lower declared value must not pull composer down below that.
	for declared, want := range map[string]string{
		"512M":       "1536M",
		"1048576K":   "1536M",
		"1536M":      "1536M",
		"2G":         "2G",
		"-1":         "-1",
		"4294967296": "4294967296",
	} {
		t.Run("declared "+declared, func(t *testing.T) {
			t.Setenv("COMPOSER_MEMORY_LIMIT", "")
			fw := &config.Framework{PHP: config.FrameworkPHP{CLIIni: map[string]string{"memory_limit": declared}}}
			got := MemoryLimitEnv(fw)
			if len(got) != 1 || got[0] != "COMPOSER_MEMORY_LIMIT="+want {
				t.Errorf("got %q, want [COMPOSER_MEMORY_LIMIT=%s]", got, want)
			}
		})
	}
	t.Run("host value wins", func(t *testing.T) {
		t.Setenv("COMPOSER_MEMORY_LIMIT", "-1")
		got := MemoryLimitEnv(withLimit)
		if len(got) != 1 || got[0] != "COMPOSER_MEMORY_LIMIT=-1" {
			t.Errorf("got %q, want [COMPOSER_MEMORY_LIMIT=-1]", got)
		}
	})
	t.Run("nothing declared", func(t *testing.T) {
		t.Setenv("COMPOSER_MEMORY_LIMIT", "")
		if got := MemoryLimitEnv(&config.Framework{}); len(got) != 0 {
			t.Errorf("got %q, want none", got)
		}
		if got := MemoryLimitEnv(nil); len(got) != 0 {
			t.Errorf("nil framework: got %q, want none", got)
		}
	})
	t.Run("unsafe value is dropped", func(t *testing.T) {
		t.Setenv("COMPOSER_MEMORY_LIMIT", "")
		bad := &config.Framework{PHP: config.FrameworkPHP{CLIIni: map[string]string{"memory_limit": "1G\n"}}}
		if got := MemoryLimitEnv(bad); len(got) != 0 {
			t.Errorf("got %q, want none", got)
		}
	})
}
