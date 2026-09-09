package nginx

import (
	"strings"
	"testing"
	"text/template"
)

// renderWorkerProxyVhost renders the plain vhost template with the proxies given.
func renderWorkerProxyVhost(t *testing.T, proxies []VhostProxy) string {
	t.Helper()
	tmplData, err := GetTemplate("vhost.conf.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("vhost").Parse(string(tmplData))
	if err != nil {
		t.Fatal(err)
	}
	out, err := renderVhost(tmpl, VhostData{
		Domain:       "app.test",
		ServerNames:  "app.test",
		Path:         "/home/user/app",
		PublicDir:    "public",
		FPMContainer: "lerd-php85-fpm",
		UpstreamHost: "169.254.1.2",
		Proxies:      proxies,
	})
	if err != nil {
		t.Fatalf("renderVhost: %v", err)
	}
	return string(out)
}

// A site can run two proxied workers, and each gets its own location and port
// rather than the first one capturing the site.
func TestVhost_RendersEveryWorkerProxy(t *testing.T) {
	conf := renderWorkerProxyVhost(t, []VhostProxy{
		{Paths: []string{"/app"}, Port: 8080},
		{Paths: []string{"/build"}, Port: 5173, OnHost: true},
	})

	if strings.Count(conf, "location ~ ^") != 2 {
		t.Errorf("locations = %d, want one per proxy\n%s", strings.Count(conf, "location ~ ^"), conf)
	}
	if !strings.Contains(conf, "proxy_pass http://$proxybackend:8080;") {
		t.Error("the container worker lost its port")
	}
	if !strings.Contains(conf, "proxy_pass http://$proxybackend:5173;") {
		t.Error("the host worker lost its port")
	}
}

// A host worker listens on the host, so proxying it to the FPM container
// reaches nothing. The upstream the dev server already uses is the right one.
func TestVhost_HostProxyTargetsTheHost(t *testing.T) {
	conf := renderWorkerProxyVhost(t, []VhostProxy{{Paths: []string{"/build"}, Port: 5173, OnHost: true}})

	if !strings.Contains(conf, `set $proxybackend "169.254.1.2";`) {
		t.Errorf("host proxy did not target the host upstream\n%s", conf)
	}
	if strings.Contains(conf, `set $proxybackend "lerd-php85-fpm";`) {
		t.Error("host proxy still targets the FPM container")
	}
}

// A container worker keeps pointing at the site's FPM container.
func TestVhost_ContainerProxyTargetsTheContainer(t *testing.T) {
	conf := renderWorkerProxyVhost(t, []VhostProxy{{Paths: []string{"/app"}, Port: 8080}})

	if !strings.Contains(conf, `set $proxybackend "lerd-php85-fpm";`) {
		t.Errorf("container proxy lost its backend\n%s", conf)
	}
}
