package nginx

import (
	"strings"
	"testing"
	"text/template"
)

// A worker proxy names either the site's FPM container or the host, because on
// the container runtime those are different places. Under the native runtime
// they are not: the worker runs on the host and so does PHP, so both have to
// reach the host or the proxied path answers nothing.
//
// It works today because a native site's FPM upstream is the host gateway, so
// the container branch already points at the host. That is worth pinning: a
// change to how a native site's upstream is resolved would otherwise send
// every proxied worker at a container that is not there, quietly.
func TestProxyReachesTheWorkerUnderNative(t *testing.T) {
	// Driven through the template with the upstreams a native site is given,
	// so this asserts what nginx is handed rather than how it was decided.
	tmplData, err := GetTemplate("vhost.conf.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("vhost").Parse(string(tmplData))
	if err != nil {
		t.Fatal(err)
	}
	data := VhostData{
		Domain:       "shop.test",
		ServerNames:  "shop.test",
		Path:         "/tmp/shop",
		PHPVersion:   "8.4",
		FPMContainer: hostGateway,
		FPMPort:      9484,
		PublicDir:    "public",
		UpstreamHost: hostGateway,
		Proxies: []VhostProxy{
			{Paths: []string{"/app"}, Port: 8080, OnHost: false},
			{Paths: []string{"/ws"}, Port: 9090, OnHost: true},
		},
		RequestTimeout: 60,
	}
	raw, err := renderVhost(tmpl, data)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`set $proxybackend "` + hostGateway + `";`,
		"proxy_pass http://$proxybackend:8080;",
		"proxy_pass http://$proxybackend:9090;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered vhost is missing %q:\n%s", want, out)
		}
	}
	// Neither kind may name a container: there is none under this runtime.
	if strings.Contains(out, "lerd-php84-fpm") {
		t.Errorf("a native vhost must not name the FPM container:\n%s", out)
	}
}
