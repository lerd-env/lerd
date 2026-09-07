package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// nativeMode points the install at the native runtime for one test.
func nativeMode(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	// Every real install has a default version; the shim falls back to it
	// outside a project.
	cfg.PHP.DefaultVersion = "8.4"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

// Both host-proxy sites and, under the native runtime, ordinary FPM sites run
// their PHP on the host, so both reach lerd services over loopback and the
// published ports rather than container DNS.
func TestUsesLoopbackServicesUnderNative(t *testing.T) {
	nativeMode(t)
	cases := []struct {
		name string
		site *config.Site
		want bool
	}{
		{"plain fpm site", &config.Site{}, true},
		{"host proxy", &config.Site{HostPort: 3000}, true},
		{"frankenphp keeps container DNS", &config.Site{Runtime: "frankenphp"}, false},
		{"custom fpm keeps container DNS", &config.Site{Runtime: "fpm-custom"}, false},
	}
	for _, c := range cases {
		if got := usesLoopbackServices(c.site); got != c.want {
			t.Errorf("%s: usesLoopbackServices = %v, want %v", c.name, got, c.want)
		}
	}
}

// The php shim runs anywhere, not only inside a registered site. Under the
// native runtime there is no FPM container to fall back to, so a directory
// that is not a site must still get native PHP rather than starting one.
func TestNativeRuntimeVersionOutsideASite(t *testing.T) {
	nativeMode(t)
	v, ok := nativeRuntimeVersion(t.TempDir())
	if !ok {
		t.Fatal("native mode must claim a directory that is not a site")
	}
	if v == "" {
		t.Error("expected a PHP version to run with")
	}
}

func TestShellRefusesUnderNative(t *testing.T) {
	nativeMode(t)
	err := nativeShellRefusal(t.TempDir())
	if err == nil {
		t.Fatal("expected a refusal under the native runtime")
	}
	for _, want := range []string{"native", "container"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Errorf("error should explain the runtime, got: %v", err)
		}
	}
}

func TestImageOnlyCommandsRefuseUnderNative(t *testing.T) {
	nativeMode(t)
	for _, cmd := range []string{"php:ext", "php:pkg"} {
		err := nativeImageCommandRefusal(cmd)
		if err == nil {
			t.Errorf("%s must refuse under the native runtime", cmd)
			continue
		}
		if !strings.Contains(err.Error(), cmd) {
			t.Errorf("the refusal should name the command, got: %v", err)
		}
	}
}
