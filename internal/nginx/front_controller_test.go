package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const (
	frontControllerLocation = `location ~ ^/index\.php(/|$) {`
	frontControllerRewrite  = "location ~ \\.php$ {\n        rewrite ^ /index.php last;\n    }"
)

// assertFrontController checks that only index.php reaches FPM, with PATH_INFO
// split off, and that every other .php URL is rewritten onto it rather than
// served from disk.
func assertFrontController(t *testing.T, content string) {
	t.Helper()
	for _, want := range []string{
		frontControllerLocation,
		`fastcgi_split_path_info ^(.+\.php)(/.*)$;`,
		"fastcgi_param PATH_INFO $fastcgi_path_info;",
		frontControllerRewrite,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in:\n%s", want, content)
		}
	}
	// Regex locations match in file order, so the rewrite placed first would
	// loop index.php back onto itself.
	if strings.Index(content, frontControllerRewrite) < strings.Index(content, frontControllerLocation) {
		t.Errorf("rewrite location precedes the index.php location:\n%s", content)
	}
}

func frontControllerProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := config.SaveProjectConfig(dir, &config.ProjectConfig{FrontController: true}); err != nil {
		t.Fatalf("SaveProjectConfig: %v", err)
	}
	return dir
}

func TestGenerateVhost_frontControllerFromProject(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: frontControllerProject(t)}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	assertFrontController(t, readConf(t, filepath.Join(confD, "app.test.conf")))
}

func TestGenerateSSLVhost_frontControllerFromProject(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: frontControllerProject(t)}
	if err := GenerateSSLVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateSSLVhost: %v", err)
	}
	assertFrontController(t, readConf(t, filepath.Join(confD, "app.test-ssl.conf")))
}

func TestGenerateVhost_frontControllerOffByDefault(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: t.TempDir()}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "app.test.conf"))
	if !strings.Contains(content, "location ~ \\.php$ {\n        set $fpm") {
		t.Errorf("expected the generic php location in:\n%s", content)
	}
	for _, bad := range []string{frontControllerLocation, "rewrite ^ /index.php last;", "fastcgi_split_path_info"} {
		if strings.Contains(content, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, content)
		}
	}
}

func TestGenerateVhost_frontControllerFromFramework(t *testing.T) {
	confD := setupConfD(t)
	store := filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd", "frameworks")
	if err := os.MkdirAll(store, 0755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	fw := "name: glpi\nlabel: GLPI\npublic_dir: public\nfront_controller: true\n"
	if err := os.WriteFile(filepath.Join(store, "glpi.yaml"), []byte(fw), 0644); err != nil {
		t.Fatalf("write framework: %v", err)
	}
	site := config.Site{Name: "itsm", Domains: []string{"itsm.test"}, Path: t.TempDir(), Framework: "glpi"}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	assertFrontController(t, readConf(t, filepath.Join(confD, "itsm.test.conf")))
}

// A worktree reads its own checkout's .lerd.yaml, the same as its timeout.
func TestGenerateWorktreeVhost_frontControllerFromProject(t *testing.T) {
	_, wt := setupWorktreeEnv(t, "")
	if err := config.SaveProjectConfig(wt, &config.ProjectConfig{FrontController: true}); err != nil {
		t.Fatalf("SaveProjectConfig: %v", err)
	}
	if err := GenerateWorktreeSSLVhost("feature.shop.test", wt, "8.4", "shop.test", "shop", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeSSLVhost: %v", err)
	}
	assertFrontController(t, readWorktreeConf(t, "feature.shop.test"))
}
