package nginx

import (
	"strings"
	"testing"
	"text/template"
)

// The dev server's whole point is one prefix carrying both assets and the
// hot-reload socket, so the location has to keep the upgrade headers.
func TestRenderVhost_devServerPrefixProxiesWithUpgrade(t *testing.T) {
	tmpl := sslVhostTemplate(t)
	data := VhostData{
		Domain:        "myapp.test",
		ServerNames:   "myapp.test",
		Path:          "/srv/myapp",
		PublicDir:     "public",
		FPMContainer:  "lerd-php84-fpm",
		CertDomain:    "myapp.test",
		UpstreamHost:  "192.0.2.1",
		DevServerBase: "/@lerd-vite/",
		DevServerPort: 5173,
	}

	out, err := renderVhost(tmpl, data)
	if err != nil {
		t.Fatalf("renderVhost: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"location ^~ /@lerd-vite/ {",
		"proxy_pass http://192.0.2.1:5173;",
		`proxy_set_header Upgrade $http_upgrade;`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vhost missing %q:\n%s", want, got)
		}
	}
}

// A site with no dev server pinned must render exactly as it did before.
func TestRenderVhost_noDevServerNoLocations(t *testing.T) {
	tmpl := sslVhostTemplate(t)
	out, err := renderVhost(tmpl, VhostData{
		Domain:       "myapp.test",
		ServerNames:  "myapp.test",
		Path:         "/srv/myapp",
		PublicDir:    "public",
		FPMContainer: "lerd-php84-fpm",
		CertDomain:   "myapp.test",
	})
	if err != nil {
		t.Fatalf("renderVhost: %v", err)
	}
	if strings.Contains(string(out), "location ^~ ") {
		t.Errorf("unexpected dev server location:\n%s", out)
	}
}

func sslVhostTemplate(t *testing.T) *template.Template {
	t.Helper()
	raw, err := GetTemplate("vhost-ssl.conf.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("vhost").Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}
