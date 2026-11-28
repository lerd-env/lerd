package config

import "testing"

// nginx moves to the host with native PHP, unless kept in its container: with
// PHP in containers the vhosts reach FPM by container name, which a host process
// cannot resolve, and off macOS there is no VM crossing to save.
func TestNginxRuntimeMode(t *testing.T) {
	cases := []struct {
		name, php, nginx, goos, want string
	}{
		{"native PHP moves nginx too", "native", "", "darwin", NginxRuntimeHost},
		{"kept in its container on request", "native", "container", "darwin", NginxRuntimeContainer},
		{"PHP in containers", "container", "", "darwin", NginxRuntimeContainer},
		{"off macOS", "native", "", "linux", NginxRuntimeContainer},
		{"unknown value moves with PHP", "native", "nonsense", "darwin", NginxRuntimeHost},
	}
	for _, c := range cases {
		cfg := &GlobalConfig{}
		cfg.PHP.Runtime = c.php
		cfg.Nginx.Runtime = c.nginx
		if got := cfg.nginxRuntimeModeOn(c.goos, "arm64"); got != c.want {
			t.Errorf("%s: NginxRuntimeMode = %q, want %q", c.name, got, c.want)
		}
	}
	var nilCfg *GlobalConfig
	if got := nilCfg.nginxRuntimeModeOn("darwin", "arm64"); got != NginxRuntimeContainer {
		t.Errorf("nil config = %q, want container", got)
	}
}
