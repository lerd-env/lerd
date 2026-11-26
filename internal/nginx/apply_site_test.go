package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// ApplySite replaces the per-kind Generate*/Install pairs every caller used to
// pick between, so for each kind and both schemes it must leave conf.d exactly
// as that pair did.
func TestApplySite_writesWhatTheKindsOwnGeneratorWrote(t *testing.T) {
	fp := fpmSite()
	fp.Runtime = "frankenphp"
	kinds := []struct {
		name   string
		site   config.Site
		legacy func(s config.Site) error
	}{
		{"fpm", fpmSite(), func(s config.Site) error { return GenerateVhost(s, s.PHPVersion) }},
		{"fpm-secured", secured(fpmSite()), func(s config.Site) error {
			return installed(s, GenerateSSLVhost(s, s.PHPVersion))
		}},
		{"frankenphp", fp, GenerateFrankenPHPVhost},
		{"frankenphp-secured", secured(fp), func(s config.Site) error { return installed(s, GenerateFrankenPHPSSLVhost(s)) }},
		{"custom", customSite(), GenerateCustomVhost},
		{"custom-secured", secured(customSite()), func(s config.Site) error { return installed(s, GenerateCustomSSLVhost(s)) }},
		{"hostproxy", hostProxySite(), GenerateHostProxyVhost},
		{"hostproxy-secured", secured(hostProxySite()), func(s config.Site) error { return installed(s, GenerateHostProxySSLVhost(s)) }},
		{"worker-subdomain-secured", config.Site{}, nil},
	}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			site, legacy := k.site, k.legacy
			if legacy == nil {
				site = secured(proxySite(t, "app", &config.WorkerProxy{Path: "/app", Subdomain: "docs", DefaultPort: 5180}))
				site.PHPVersion = "8.4"
				legacy = func(s config.Site) error { return installed(s, GenerateSSLVhost(s, s.PHPVersion)) }
			}
			confD := setupConfD(t)
			must(t, legacy(site))
			want := readDir(t, confD)
			must(t, os.RemoveAll(confD))

			must(t, ApplySite(site))
			got := readDir(t, confD)
			if len(got) != len(want) {
				t.Fatalf("ApplySite wrote %v, the generator wrote %v", keys(got), keys(want))
			}
			for name, w := range want {
				if got[name] != w {
					t.Errorf("%s: %s", name, firstDifference(w, got[name]))
				}
			}
		})
	}
}

// A site that names no PHP version is served by the default version's pool,
// never by an unversioned lerd-php-fpm that no unit ever starts.
func TestApplySite_siteWithoutAVersionGetsTheDefaultPool(t *testing.T) {
	confD := setupConfD(t)
	site := fpmSite()
	site.PHPVersion = ""
	must(t, ApplySite(site))
	conf := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	want := "lerd-php" + phpShort(sitePHPVersion(site)) + "-fpm"
	if sitePHPVersion(site) == "" || !strings.Contains(conf, want) {
		t.Errorf("expected the default pool %s in:\n%s", want, conf)
	}
}

func secured(s config.Site) config.Site {
	s.Secured = true
	return s
}

func installed(s config.Site, err error) error {
	if err != nil {
		return err
	}
	return InstallSSLVhost(s.PrimaryDomain())
}

func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
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
