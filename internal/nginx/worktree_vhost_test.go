package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
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
  fastcgi_params:
    SHOP_ROOT: "{{root}}"
    SHOP_PUBLIC: "{{public}}"
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

// A framework's fastcgi params reach every generated vhost through the real
// generators, expanded to that checkout's own path: a worktree must not hand PHP
// the parent's.
func TestGeneratorsRenderFrameworkFastCGIParams(t *testing.T) {
	parent, wt := setupWorktreeEnv(t, "")
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatalf("FindSite: %v", err)
	}
	for name, tc := range map[string]struct {
		generate func() error
		domain   string
		path     string
	}{
		"site http":     {func() error { return GenerateVhost(*site, "8.4") }, "shop.test", parent},
		"site https":    {func() error { return GenerateSSLVhost(*site, "8.4") }, "shop.test-ssl", parent},
		"worktree http": {func() error { return GenerateWorktreeVhost("feature.shop.test", wt, "8.4", "shop", "feature") }, "feature.shop.test", wt},
		"worktree https": {func() error {
			return GenerateWorktreeSSLVhost("feature.shop.test", wt, "8.4", "shop.test", "shop", "feature")
		}, "feature.shop.test", wt},
	} {
		if err := tc.generate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := readWorktreeConf(t, tc.domain)
		for _, want := range []string{
			`fastcgi_param SHOP_ROOT "` + tc.path + `";`,
			`fastcgi_param SHOP_PUBLIC "` + tc.path + `/pub";`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q in:\n%s", name, want, out)
			}
		}
	}
}

// setupCustomWorktreeEnv registers a custom-container parent site and its
// worktree checkout, so the custom-container branch vhost can be rendered
// without a PHP-FPM upstream.
func setupCustomWorktreeEnv(t *testing.T) (parentPath, worktreePath string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	parentPath = filepath.Join(tmp, "nestapp")
	worktreePath = filepath.Join(tmp, "nestapp-feature")
	for _, p := range []string{parentPath, worktreePath} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	lerdDir := filepath.Join(tmp, "lerd")
	if err := os.MkdirAll(lerdDir, 0755); err != nil {
		t.Fatalf("mkdir lerd: %v", err)
	}
	sites := `sites:
- name: nestapp
  domains:
    - nestapp.test
  path: ` + parentPath + `
  container_port: 8000
`
	if err := os.WriteFile(filepath.Join(lerdDir, "sites.yaml"), []byte(sites), 0644); err != nil {
		t.Fatalf("write sites.yaml: %v", err)
	}
	return parentPath, worktreePath
}

// A worktree of a custom-container site must proxy to its own container, not
// fall back to fastcgi_pass on the shared PHP-FPM pool.
func TestGenerateWorktreeCustomVhostProxiesToBranchContainer(t *testing.T) {
	_, wt := setupCustomWorktreeEnv(t)

	if err := GenerateWorktreeCustomVhostFor("feature.nestapp.test", wt, "nestapp.test", "nestapp", "feature", false); err != nil {
		t.Fatalf("GenerateWorktreeCustomVhostFor: %v", err)
	}
	out := readWorktreeConf(t, "feature.nestapp.test")

	if !strings.Contains(out, `set $backend "lerd-custom-nestapp-feature";`) {
		t.Errorf("branch vhost does not target the branch container:\n%s", out)
	}
	if !strings.Contains(out, "proxy_pass http://$backend:8000;") {
		t.Errorf("branch vhost does not proxy to the custom port:\n%s", out)
	}
	if strings.Contains(out, "fastcgi_pass") {
		t.Errorf("custom-container worktree fell back to fastcgi_pass:\n%s", out)
	}
}

// The secured worktree variant reuses the parent's wildcard certificate and
// keeps proxying to the branch container.
func TestGenerateWorktreeCustomSSLVhostUsesParentCert(t *testing.T) {
	_, wt := setupCustomWorktreeEnv(t)

	if err := GenerateWorktreeCustomVhostFor("feature.nestapp.test", wt, "nestapp.test", "nestapp", "feature", true); err != nil {
		t.Fatalf("GenerateWorktreeCustomVhostFor(ssl): %v", err)
	}
	out := readWorktreeConf(t, "feature.nestapp.test")

	if !strings.Contains(out, "ssl_certificate /etc/nginx/certs/nestapp.test.crt;") {
		t.Errorf("worktree SSL vhost does not use the parent wildcard cert:\n%s", out)
	}
	if !strings.Contains(out, `set $backend "lerd-custom-nestapp-feature";`) {
		t.Errorf("worktree SSL vhost does not target the branch container:\n%s", out)
	}
}
