package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

// ── siteNameAndDomain ────────────────────────────────────────────────────────

func TestSiteNameAndDomain(t *testing.T) {
	cases := []struct {
		dirName    string
		tld        string
		wantName   string
		wantDomain string
	}{
		{"myapp", "test", "myapp", "myapp.test"},
		{"MyApp", "test", "myapp", "myapp.test"},
		{"admin.starlane.com", "test", "admin-starlane", "admin-starlane.test"},
		{"my.project.io", "test", "my-project", "my-project.test"},
		{"shop.co", "test", "shop", "shop.test"},
		{"api.dev", "test", "api", "api.test"},
		{"plain", "local", "plain", "plain.local"},
		{"has.dots.net", "test", "has-dots", "has-dots.test"},
	}
	for _, c := range cases {
		gotName, gotDomain := siteops.SiteNameAndDomain(c.dirName, c.tld)
		if gotName != c.wantName {
			t.Errorf("siteNameAndDomain(%q, %q) name = %q, want %q", c.dirName, c.tld, gotName, c.wantName)
		}
		if gotDomain != c.wantDomain {
			t.Errorf("siteNameAndDomain(%q, %q) domain = %q, want %q", c.dirName, c.tld, gotDomain, c.wantDomain)
		}
	}
}

// ── checkExtensions ──────────────────────────────────────────────────────────

func TestCheckExtensions(t *testing.T) {
	bundled := []string{"opcache", "redis", "mbstring"}
	installed := []string{"imap"}

	cases := []struct {
		name            string
		detected        []string
		wantMissing     []string
		wantUnavailable []extUnavailable
		wantMisnamed    []extMismatch
	}{
		{
			// #842: the extension is in the image, but composer publishes it as
			// ext-zend-opcache, so composer install fails its platform check.
			name:         "ext-opcache is present but misnamed",
			detected:     []string{"opcache"},
			wantMisnamed: []extMismatch{{Required: "opcache", Platform: "zend-opcache"}},
		},
		{
			name:     "ext-zend-opcache resolves against the bundled opcache",
			detected: []string{"zend-opcache"},
		},
		{
			name:        "a genuinely absent extension is still reported",
			detected:    []string{"snmp"},
			wantMissing: []string{"snmp"},
		},
		{
			// ext/random is core only from 8.2, so php:ext add can never build it
			// on an older site. Offering the install is a dead-end image rebuild.
			name:            "an extension this PHP version cannot ship is explained, not offered for install",
			detected:        []string{"random"},
			wantUnavailable: []extUnavailable{{Required: "random", Since: "8.2"}},
		},
		{
			name:     "a custom-installed extension is not missing",
			detected: []string{"imap"},
		},
		{
			name:     "a plainly bundled extension is silent",
			detected: []string{"redis", "mbstring"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			missing, unavailable, misnamed := checkExtensions(c.detected, bundled, installed)
			if !reflect.DeepEqual(missing, c.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, c.wantMissing)
			}
			if !reflect.DeepEqual(unavailable, c.wantUnavailable) {
				t.Errorf("unavailable = %v, want %v", unavailable, c.wantUnavailable)
			}
			if !reflect.DeepEqual(misnamed, c.wantMisnamed) {
				t.Errorf("misnamed = %v, want %v", misnamed, c.wantMisnamed)
			}
		})
	}
}

// ── RegisterProject skips custom containers ─────────────────────────────────

func TestRegisterProject_SkipsCustomContainer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Create a project directory with a composer.json so it looks like a PHP project.
	projectDir := filepath.Join(t.TempDir(), "nestapp")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "composer.json"), []byte(`{}`), 0644)

	// Pre-register the site as a custom container.
	config.AddSite(config.Site{
		Name:          "nestapp",
		Domains:       []string{"nestapp.test"},
		Path:          projectDir,
		ContainerPort: 3000,
	})

	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	cfg.PHP.DefaultVersion = "8.4"

	registered, err := RegisterProject(projectDir, cfg)
	if err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}
	if registered {
		t.Error("RegisterProject should return false for a custom container site")
	}
}
