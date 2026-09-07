package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A bucket row is only useful if it says where the bucket actually is, and the
// answer changes the moment the service gets a domain: the container name it is
// otherwise reached at resolves in no browser.
func TestEntityBaseURL_FollowsTheServiceDomain(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["rustfs"] = config.ServiceConfig{Port: 9000}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if got := entityBaseURL("rustfs"); got != "http://localhost:9000" {
		t.Errorf("base = %q, want the published loopback port", got)
	}

	sc := cfg.Services["rustfs"]
	sc.Domain = "rustfs.test"
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if got := entityBaseURL("rustfs"); got != "https://rustfs.test" {
		t.Errorf("base = %q, want the service domain", got)
	}
}

func TestEntityBaseURL_NoPortsNoURL(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if got := entityBaseURL("not-a-service"); got != "" {
		t.Errorf("base = %q, want empty for a service with nothing published", got)
	}
}
