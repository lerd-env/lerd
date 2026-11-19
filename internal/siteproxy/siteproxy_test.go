package siteproxy

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestPaths_normaliseTheDeclaredPrefixes(t *testing.T) {
	cases := []struct {
		proxy config.WorkerProxy
		want  []string
	}{
		{config.WorkerProxy{Path: "/app/"}, []string{"/app"}},
		{config.WorkerProxy{Path: "app"}, []string{"/app"}},
		{config.WorkerProxy{Path: "/"}, []string{""}},
		{config.WorkerProxy{Path: "/old", Paths: []string{"/a", "", "b/"}}, []string{"/a", "/b"}},
		{config.WorkerProxy{}, nil},
	}
	for _, c := range cases {
		if got := Paths(&c.proxy); !slices.Equal(got, c.want) {
			t.Errorf("Paths(%+v) = %q, want %q", c.proxy, got, c.want)
		}
	}
}

func TestPort_pinnedThenEnvThenDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("REVERB_PORT=8181\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Path: dir, WorkerPorts: map[string]int{"ws": 5191}}
	cases := []struct {
		name string
		np   config.NamedProxy
		want int
	}{
		{"pinned", config.NamedProxy{Worker: "ws", Proxy: &config.WorkerProxy{Port: "pinned", DefaultPort: 5180}}, 5191},
		{"from env", config.NamedProxy{Worker: "reverb", Proxy: &config.WorkerProxy{PortEnvKey: "REVERB_PORT", DefaultPort: 8080}}, 8181},
		{"default", config.NamedProxy{Worker: "x", Proxy: &config.WorkerProxy{DefaultPort: 5173}}, 5173},
		{"fallback", config.NamedProxy{Worker: "x", Proxy: &config.WorkerProxy{}}, 8080},
	}
	for _, c := range cases {
		if got := Port(site, c.np); got != c.want {
			t.Errorf("%s: Port = %d, want %d", c.name, got, c.want)
		}
	}
}
