package cli

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
)

func TestSecureCmd_RefusesWhenDNSDisabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg := &config.GlobalConfig{}
	cfg.DNS.Enabled = false
	cfg.DNS.TLD = "localhost"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if err := config.AddSite(config.Site{Name: "myapp", Path: tmp, Domains: []string{"myapp.localhost"}}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	err := toggleSecureCmd([]string{"myapp"}, true)
	if !errors.Is(err, certs.ErrDNSDisabled) {
		t.Fatalf("toggleSecureCmd err = %v, want ErrDNSDisabled", err)
	}
}
