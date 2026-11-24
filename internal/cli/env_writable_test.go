package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestWritesEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, fw := range []*config.Framework{
		{Name: "bare", Label: "Bare"},
		{Name: "fileonly", Label: "File", Env: config.FrameworkEnvConf{File: ".env"}},
		{Name: "mapped", Label: "Mapped", Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{"redis": {}}}},
	} {
		if err := config.SaveFramework(fw); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		framework, file string
		want            bool
	}{
		{"", "", false},
		{"", ".env", false},
		{"missing", ".env", false},
		{"bare", ".env", false},
		{"mapped", "", true},
		{"fileonly", "", false},
		{"fileonly", ".env", true},
		{"fileonly", ".env.example", true},
	} {
		dir := t.TempDir()
		if c.file != "" {
			if err := os.WriteFile(filepath.Join(dir, c.file), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := WritesEnv(&config.Site{Framework: c.framework, Path: dir}); got != c.want {
			t.Errorf("framework %q with %q: %v, want %v", c.framework, c.file, got, c.want)
		}
	}
}
