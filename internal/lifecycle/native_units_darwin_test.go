package lifecycle

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Under the native runtime PHP runs on the host, so the shared FPM containers
// have nothing to serve. Leaving them in the core set means every `lerd start`
// brings back the containers the runtime switch just stopped, which quietly
// undoes the whole point of the mode.
func TestCoreUnitsExcludesFPMUnderNative(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	cfg.PHP.DefaultVersion = "8.4"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	for _, u := range CoreUnits() {
		if strings.HasPrefix(u, "lerd-php") && strings.HasSuffix(u, "-fpm") {
			t.Errorf("native runtime must not start %s", u)
		}
	}
	// nginx still serves every site and must stay.
	var hasNginx bool
	for _, u := range CoreUnits() {
		if u == "lerd-nginx" {
			hasNginx = true
		}
	}
	if !hasNginx {
		t.Error("nginx must stay in the core units; it still serves every site")
	}
}

// Container mode is untouched: the default version's FPM is always there so the
// php and composer shims work even with no sites registered.
func TestCoreUnitsKeepsFPMInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.DefaultVersion = "8.4"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	units := CoreUnits()
	var hasNginx bool
	for _, u := range units {
		if u == "lerd-nginx" {
			hasNginx = true
		}
	}
	if !hasNginx {
		t.Fatalf("expected nginx in %v", units)
	}
}

// Site restore ensures an FPM container per site PHP version, which is the
// other path that brings the containers back. It has to respect the runtime
// too, or `lerd start` undoes the switch every time.
func TestFPMContainersWantedOnlyInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if FPMContainersWanted() {
		t.Error("native runtime must not want FPM containers")
	}

	cfg.PHP.Runtime = config.PHPRuntimeContainer
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if !FPMContainersWanted() {
		t.Error("container runtime must want FPM containers")
	}
}
