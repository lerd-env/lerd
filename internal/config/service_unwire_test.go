package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/envfile"
)

func redisStoreDef() *Framework {
	return &Framework{
		Name:      "shopfw",
		Label:     "Shop",
		PublicDir: "public",
		Detect:    []FrameworkRule{{File: "shop"}},
		Env: FrameworkEnvConf{
			File:        ".env",
			ExampleFile: ".env.example",
			Services: map[string]FrameworkServiceDef{
				"redis": {Vars: []string{"REDIS_HOST=lerd-redis", "REDIS_PORT=6379", "REDIS_PASSWORD="}},
			},
		},
	}
}

// The framework's keys for the service go back to the project's own example
// values; one the example lacks keeps its value unless it names the container,
// which is cleared, so nothing in the file reaches the service any more.
func TestUnwireProjectService(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, redisStoreDef())
	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: shopfw\n")
	writeProjectFile(t, dir, ".env.example", "REDIS_HOST=127.0.0.1\nREDIS_PASSWORD=null\n")
	writeProjectFile(t, dir, ".env", "APP_NAME=Shop\nREDIS_HOST=lerd-redis\nREDIS_PORT=6379\nREDIS_PASSWORD=\nBROADCAST_URL=redis://lerd-redis:6379\nCACHE_STORE=redis\n")

	if err := UnwireProjectService(dir, "redis"); err != nil {
		t.Fatal(err)
	}

	got := envfile.ReadValues(filepath.Join(dir, ".env"))
	want := map[string]string{
		"APP_NAME":       "Shop",
		"REDIS_HOST":     "127.0.0.1",
		"REDIS_PORT":     "6379",
		"REDIS_PASSWORD": "null",
		"BROADCAST_URL":  "",
		"CACHE_STORE":    "redis",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if envfile.ReferencesContainer(string(data), "redis") {
		t.Errorf(".env still reaches lerd-redis:\n%s", data)
	}
}

// Only keys the file already has are touched; un-wiring never adds one.
func TestUnwireProjectServiceAddsNoKeys(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, redisStoreDef())
	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: shopfw\n")
	writeProjectFile(t, dir, ".env", "REDIS_HOST=lerd-redis\n")

	if err := UnwireProjectService(dir, "redis"); err != nil {
		t.Fatal(err)
	}
	got := envfile.ReadValues(filepath.Join(dir, ".env"))
	if _, ok := got["REDIS_PORT"]; ok {
		t.Errorf("REDIS_PORT was added: %v", got)
	}
}
