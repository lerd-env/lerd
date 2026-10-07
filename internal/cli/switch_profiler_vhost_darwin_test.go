package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The profiler vhost is not a site's, so the runtime switch's sweep over sites
// never rewrote it. After moving to native PHP it still named the FPM container
// nothing ran in any more, and the SPX dashboard answered 502.
func TestRuntimeSwitchRegeneratesProfilerVhost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, _ := config.LoadGlobal()
	cfg.PHP.DefaultVersion = "8.4"
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	regenerateProfilerVhost()

	data, err := os.ReadFile(filepath.Join(tmp, "lerd", "nginx", "conf.d", "_profiler.conf"))
	if err != nil {
		t.Fatalf("profiler vhost not written: %v", err)
	}
	if strings.Contains(string(data), "lerd-php84-fpm") {
		t.Errorf("profiler vhost still names the FPM container under the native runtime:\n%s", data)
	}
}
