package config

import (
	"os"
	"path/filepath"
	"testing"
)

func viteLikeFramework() *Framework {
	paths := []string{"/themes/{theme}/assets/dist"}
	return &Framework{
		Name: "winterish",
		Workers: map[string]FrameworkWorker{
			"vite": {
				Command:     "php artisan vite:watch theme-demo",
				TuneCommand: "php artisan vite:watch theme-{theme}",
				Proxy:       &WorkerProxy{Paths: paths, Port: "pinned"},
			},
		},
	}
}

// The server answers under a base named after the package its command names, so
// the proxy has to follow the same value rather than the definition's example.
func TestDetectProxies_fillsPathsFromTheTuneDefault(t *testing.T) {
	fw := viteLikeFramework()
	got := fw.DetectProxies(t.TempDir())
	if len(got) != 1 {
		t.Fatalf("got %d proxies, want 1", len(got))
	}
	if p := got[0].Proxy.Paths[0]; p != "/themes/demo/assets/dist" {
		t.Errorf("path = %q, want the tune default filled in", p)
	}
	if declared := fw.Workers["vite"].Proxy.Paths[0]; declared != "/themes/{theme}/assets/dist" {
		t.Errorf("the definition was mutated: %q", declared)
	}
}

func TestDetectProxies_projectValueWinsOverTheDefault(t *testing.T) {
	dir := t.TempDir()
	yaml := "framework: winterish\nworker_options:\n  vite:\n    theme: aurora\n"
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	got := viteLikeFramework().DetectProxies(dir)
	if len(got) != 1 {
		t.Fatalf("got %d proxies, want 1", len(got))
	}
	if p := got[0].Proxy.Paths[0]; p != "/themes/aurora/assets/dist" {
		t.Errorf("path = %q, want the project's own theme", p)
	}
}

// A placeholder nothing supplies stays put: routing a proxy at a literal
// "{theme}" is wrong, but quietly routing it at the wrong package is worse.
func TestExpandTunePlaceholders_leavesAnUnknownTokenAlone(t *testing.T) {
	got := ExpandTunePlaceholders("/themes/{theme}/assets/dist", map[string]string{"other": "x"})
	if got != "/themes/{theme}/assets/dist" {
		t.Errorf("got %q, want the token left in place", got)
	}
}

// A definition with no placeholders keeps the path it declared.
func TestDetectProxies_leavesAPlainPathAlone(t *testing.T) {
	fw := &Framework{
		Name: "plain",
		Workers: map[string]FrameworkWorker{
			"reverb": {
				Command: "php artisan reverb:start",
				Proxy:   &WorkerProxy{Paths: []string{"/app"}},
			},
		},
	}
	got := fw.DetectProxies(t.TempDir())
	if len(got) != 1 || got[0].Proxy.Paths[0] != "/app" {
		t.Errorf("got %+v, want /app untouched", got)
	}
}
