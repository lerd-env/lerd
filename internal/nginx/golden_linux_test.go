package nginx

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The golden files pin every vhost lerd writes, byte for byte, so moving the
// nginx code behind the web server seam cannot change what nginx serves.
// Linux only: the host-proxy upstream differs on macOS.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden from the current renders")

func TestGoldenRenders(t *testing.T) {
	cases := []struct {
		name   string
		render func(t *testing.T)
	}{
		{"fpm-http", func(t *testing.T) { must(t, GenerateVhost(fpmSite(), "8.4")) }},
		{"fpm-https", func(t *testing.T) { must(t, GenerateSSLVhost(fpmSite(), "8.4")) }},
		{"fpm-https-installed", func(t *testing.T) {
			must(t, GenerateSSLVhost(fpmSite(), "8.4"))
			must(t, InstallSSLVhost("myapp.test"))
		}},
		{"fpm-multi-domain", func(t *testing.T) {
			s := fpmSite()
			s.Domains = []string{"myapp.test", "alias.test"}
			must(t, GenerateVhost(s, "8.3"))
		}},
		{"frankenphp-http", func(t *testing.T) { must(t, GenerateFrankenPHPVhost(fpmSite())) }},
		{"frankenphp-https", func(t *testing.T) { must(t, GenerateFrankenPHPSSLVhost(fpmSite())) }},
		{"custom-http", func(t *testing.T) { must(t, GenerateCustomVhost(customSite())) }},
		{"custom-https", func(t *testing.T) { must(t, GenerateCustomSSLVhost(customSite())) }},
		{"hostproxy-http", func(t *testing.T) { must(t, GenerateHostProxyVhost(hostProxySite())) }},
		{"hostproxy-https", func(t *testing.T) { must(t, GenerateHostProxySSLVhost(hostProxySite())) }},
		{"worktree-http", func(t *testing.T) {
			must(t, GenerateWorktreeVhost("feat.myapp.test", "/srv/myapp-feat", "8.4", "myapp", "feat"))
		}},
		{"worktree-https", func(t *testing.T) {
			must(t, GenerateWorktreeSSLVhost("feat.myapp.test", "/srv/myapp-feat", "8.4", "myapp.test", "myapp", "feat"))
		}},
		{"worktree-hostproxy-https", func(t *testing.T) {
			must(t, GenerateWorktreeHostProxyVhostFor("feat.myapp.test", "/srv/myapp-feat", "myapp.test", 5173, false, true))
		}},
		{"paused-http", func(t *testing.T) { must(t, GeneratePausedVhost(fpmSite())) }},
		{"paused-https", func(t *testing.T) {
			s := fpmSite()
			s.Secured = true
			must(t, GeneratePausedVhost(s))
		}},
		{"waking-hostproxy", func(t *testing.T) { must(t, GenerateWakingVhost(hostProxySite())) }},
		{"paused-worktree-https", func(t *testing.T) {
			must(t, GeneratePausedWorktreeVhost("feat.myapp.test", "myapp.test", "/srv/paused", true))
		}},
		{"service-proxy-http", func(t *testing.T) {
			must(t, GenerateServiceProxyVhost("mail.lerd.test", "lerd-mailpit", 8025, false, false))
		}},
		{"service-proxy-https-cors", func(t *testing.T) {
			must(t, GenerateServiceProxyVhost("mail.lerd.test", "lerd-mailpit", 8025, true, true))
		}},
		{"worker-path-and-subdomain-https", func(t *testing.T) {
			s := proxySite(t, "app", &config.WorkerProxy{Path: "/app", Subdomain: "docs", DefaultPort: 5180})
			must(t, GenerateSSLVhost(s, "8.4"))
		}},
		{"base-config", func(t *testing.T) {
			must(t, EnsureNginxConfig())
			must(t, EnsureDefaultVhost())
			must(t, EnsureLerdVhost())
			must(t, EnsureProfilerVhost())
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nginxDir := filepath.Dir(setupConfD(t))
			c.render(t)
			compareGolden(t, filepath.Join("testdata", "golden", c.name), nginxDir)
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func customSite() config.Site {
	s := fpmSite()
	s.ContainerPort = 8080
	return s
}

func hostProxySite() config.Site {
	s := fpmSite()
	s.HostPort = 5173
	return s
}

// compareGolden diffs every file under got against the golden directory. The
// temp root, trust token and SPX key differ per run, so they are masked, and
// the managed-hash sidecar only hashes them.
func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	tmpRoot := filepath.Dir(filepath.Dir(filepath.Dir(got)))
	token, _ := LoadOrGenerateTrustToken()
	spxKey, _ := config.LoadOrGenerateProfilerKey()
	rendered := map[string]string{}
	filepath.WalkDir(got, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(d.Name(), ".lerd-managed-hash") {
			return err
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(got, path)
		text := strings.ReplaceAll(string(data), tmpRoot, "$TMP")
		if token != "" {
			text = strings.ReplaceAll(text, token, "$TOKEN")
		}
		if spxKey != "" {
			text = strings.ReplaceAll(text, spxKey, "$SPXKEY")
		}
		rendered[rel] = text
		return nil
	})
	if *updateGolden {
		must(t, os.RemoveAll(golden))
		for rel, data := range rendered {
			must(t, os.MkdirAll(filepath.Dir(filepath.Join(golden, rel)), 0o755))
			must(t, os.WriteFile(filepath.Join(golden, rel), []byte(data), 0o644))
		}
		return
	}
	want := map[string]string{}
	filepath.WalkDir(golden, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := os.ReadFile(path)
		rel, _ := filepath.Rel(golden, path)
		want[rel] = string(data)
		return nil
	})
	if len(want) == 0 {
		t.Fatalf("no golden files in %s; run with -update-golden on main", golden)
	}
	for rel, w := range want {
		g, ok := rendered[rel]
		if !ok {
			t.Errorf("%s: no longer written", rel)
			continue
		}
		if g != w {
			t.Errorf("%s changed: %s", rel, firstDifference(w, g))
		}
	}
	for rel := range rendered {
		if _, ok := want[rel]; !ok {
			t.Errorf("%s: newly written, not in golden", rel)
		}
	}
}
