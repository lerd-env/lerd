package config

import "testing"

func TestPHPRuntimeMode(t *testing.T) {
	cases := []struct {
		configured string
		want       string
	}{
		{"", PHPRuntimeContainer},
		{"container", PHPRuntimeContainer},
		{"native", PHPRuntimeNative},
		{"nonsense", PHPRuntimeContainer},
	}
	for _, c := range cases {
		cfg := &GlobalConfig{}
		cfg.PHP.Runtime = c.configured
		if got := cfg.PHPRuntimeMode(); got != c.want {
			t.Errorf("PHPRuntimeMode(%q) = %q, want %q", c.configured, got, c.want)
		}
	}
	// A nil config is an unconfigured install, which serves from containers.
	var nilCfg *GlobalConfig
	if got := nilCfg.PHPRuntimeMode(); got != PHPRuntimeContainer {
		t.Errorf("nil config = %q, want %q", got, PHPRuntimeContainer)
	}
}

// Native serves a site through PHP-FPM on the host. A site that is not served
// by FPM at all has nothing to move, so the mode must not claim it.
func TestSiteServedNatively(t *testing.T) {
	cases := []struct {
		name string
		site Site
		want bool
	}{
		{"plain fpm site", Site{Name: "a"}, true},
		{"frankenphp runs its own container", Site{Name: "a", Runtime: "frankenphp"}, false},
		{"custom fpm has a per-site image", Site{Name: "a", Runtime: "fpm-custom"}, false},
		{"custom container is user-defined", Site{Name: "a", ContainerPort: 8080}, false},
		{"host proxy already runs on the host", Site{Name: "a", HostPort: 3000}, false},
	}
	for _, c := range cases {
		if got := c.site.ServedNatively(PHPRuntimeNative); got != c.want {
			t.Errorf("%s: ServedNatively(native) = %v, want %v", c.name, got, c.want)
		}
		// In container mode nothing is served natively, whatever the site is.
		if c.site.ServedNatively(PHPRuntimeContainer) {
			t.Errorf("%s: must not be native in container mode", c.name)
		}
	}
}
