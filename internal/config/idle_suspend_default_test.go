package config

import (
	"os"
	"testing"
)

func TestLoadGlobal_IdleSuspendOnByDefault(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IdleSuspend.Enabled {
		t.Error("idle-suspend should be on for an install that never configured it")
	}
	if cfg.IdleSuspend.Services {
		t.Error("idle-suspend for services should stay off by default")
	}
}

func TestSaveGlobal_IdleSuspendOffSurvivesReload(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.IdleSuspend.Enabled = false
	if err := SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	invalidateGlobalCache()

	cfg, err = LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdleSuspend.Enabled {
		data, _ := os.ReadFile(GlobalConfigFile())
		t.Errorf("turning idle-suspend off should persist, saved config:\n%s", data)
	}
}

func TestLoadGlobal_IdleSuspendFromExistingFile(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"no idle_suspend key", "dns:\n  tld: test\n", true},
		{"timeout only", "idle_suspend:\n  timeout: 1h\n", true},
		{"explicit off", "idle_suspend:\n  enabled: false\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setConfigDir(t)
			if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(GlobalConfigFile(), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			invalidateGlobalCache()

			cfg, err := LoadGlobal()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.IdleSuspend.Enabled != tc.want {
				t.Errorf("enabled = %v, want %v", cfg.IdleSuspend.Enabled, tc.want)
			}
		})
	}
}
