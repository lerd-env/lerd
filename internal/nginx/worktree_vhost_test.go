package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorktreeEnv registers a parent site whose framework serves from a
// non-default public dir and declares an nginx block, then returns the parent
// and worktree checkout paths. The worktree lives beside the parent, the way
// `lerd worktree add` creates it.
func setupWorktreeEnv(t *testing.T, sitePublicDir string) (parentPath, worktreePath string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	parentPath = filepath.Join(tmp, "shop")
	worktreePath = filepath.Join(tmp, "shop-feature")
	for _, p := range []string{parentPath, worktreePath} {
		if err := os.MkdirAll(filepath.Join(p, "pub"), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	store := filepath.Join(tmp, "lerd", "frameworks")
	if err := os.MkdirAll(store, 0755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	fw := `name: magento
label: Magento
public_dir: pub
nginx:
  snippet: |
    location ~* ^/(setup|update)($|/) {
        root {{root}};
        fastcgi_pass {{fpm}}:9000;
    }
`
	if err := os.WriteFile(filepath.Join(store, "magento.yaml"), []byte(fw), 0644); err != nil {
		t.Fatalf("write framework: %v", err)
	}

	publicDir := ""
	if sitePublicDir != "" {
		publicDir = "\n  public_dir: " + sitePublicDir
	}
	sites := `sites:
- name: shop
  domains:
    - shop.test
  path: ` + parentPath + `
  php_version: "8.4"
  framework: magento` + publicDir + "\n"

	lerdDir := filepath.Join(tmp, "lerd")
	if err := os.MkdirAll(lerdDir, 0755); err != nil {
		t.Fatalf("mkdir lerd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lerdDir, "sites.yaml"), []byte(sites), 0644); err != nil {
		t.Fatalf("write sites.yaml: %v", err)
	}
	return parentPath, worktreePath
}

func readWorktreeConf(t *testing.T, domain string) string {
	t.Helper()
	conf := filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd", "nginx", "conf.d", domain+".conf")
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("read %s: %v", conf, err)
	}
	return string(b)
}

// A worktree of a site whose framework serves from "pub" must not get a
// document root of "public": nothing lives there, so every request 404s.
func TestGenerateWorktreeVhostUsesFrameworkPublicDir(t *testing.T) {
	_, wt := setupWorktreeEnv(t, "")

	if err := GenerateWorktreeVhost("feature.shop.test", wt, "8.4", "shop", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeVhost: %v", err)
	}
	out := readWorktreeConf(t, "feature.shop.test")

	if want := `root "` + wt + `/pub";`; !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
	if bad := `root "` + wt + `/public";`; strings.Contains(out, bad) {
		t.Errorf("worktree rooted at the hardcoded public dir:\n%s", out)
	}
}

// The framework's nginx block must reach the worktree vhost too, expanded
// against the worktree's own checkout rather than the parent's.
func TestGenerateWorktreeVhostRendersFrameworkNginx(t *testing.T) {
	parent, wt := setupWorktreeEnv(t, "")

	if err := GenerateWorktreeVhost("feature.shop.test", wt, "8.4", "shop", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeVhost: %v", err)
	}
	out := readWorktreeConf(t, "feature.shop.test")

	if !strings.Contains(out, `location ~* ^/(setup|update)($|/) {`) {
		t.Fatalf("framework snippet missing from worktree vhost:\n%s", out)
	}
	if want := `set $lerd_root "` + wt + `";`; !strings.Contains(out, want) {
		t.Errorf("snippet {{root}} should expand to the worktree, want %q in:\n%s", want, out)
	}
	if bad := `set $lerd_root "` + parent + `";`; strings.Contains(out, bad) {
		t.Errorf("snippet {{root}} expanded to the parent checkout:\n%s", out)
	}
	if !strings.Contains(out, "fastcgi_pass lerd-php84-fpm:9000;") {
		t.Errorf("snippet {{fpm}} unexpanded in:\n%s", out)
	}
}

// The SSL path renders from a different template and generator, so it needs the
// same coverage: a secured worktree must not lose the root or the snippet.
func TestGenerateWorktreeSSLVhostUsesFrameworkConfig(t *testing.T) {
	_, wt := setupWorktreeEnv(t, "")

	if err := GenerateWorktreeSSLVhost("feature.shop.test", wt, "8.4", "shop.test", "shop", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeSSLVhost: %v", err)
	}
	out := readWorktreeConf(t, "feature.shop.test")

	if want := `root "` + wt + `/pub";`; !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
	if !strings.Contains(out, `location ~* ^/(setup|update)($|/) {`) {
		t.Errorf("framework snippet missing from SSL worktree vhost:\n%s", out)
	}
	if !strings.Contains(out, "ssl_certificate") {
		t.Errorf("not an SSL vhost:\n%s", out)
	}
}

// A site-level public_dir (from .lerd.yaml) outranks the framework's, on the
// worktree exactly as it does on the parent.
func TestGenerateWorktreeVhostHonoursSitePublicDir(t *testing.T) {
	_, wt := setupWorktreeEnv(t, "web")

	if err := GenerateWorktreeVhost("feature.shop.test", wt, "8.4", "shop", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeVhost: %v", err)
	}
	out := readWorktreeConf(t, "feature.shop.test")

	if want := `root "` + wt + `/web";`; !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

// An unregistered parent (a worktree scanned before its site lands in the
// registry) keeps the old behaviour rather than erroring: public root, no block.
func TestGenerateWorktreeVhostUnknownParentFallsBack(t *testing.T) {
	_, wt := setupWorktreeEnv(t, "")

	if err := GenerateWorktreeVhost("feature.ghost.test", wt, "8.4", "ghost", "feature"); err != nil {
		t.Fatalf("GenerateWorktreeVhost: %v", err)
	}
	out := readWorktreeConf(t, "feature.ghost.test")

	if want := `root "` + wt + `/public";`; !strings.Contains(out, want) {
		t.Errorf("missing fallback %q in:\n%s", want, out)
	}
	if strings.Contains(out, "^/(setup|update)") {
		t.Errorf("unknown parent should carry no framework block:\n%s", out)
	}
	if !strings.Contains(out, "fastcgi_pass $fpm:9000;") {
		t.Errorf("missing shared FPM pass in:\n%s", out)
	}
}
