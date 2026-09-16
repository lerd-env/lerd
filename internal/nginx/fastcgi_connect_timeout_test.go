package nginx

import (
	"strings"
	"testing"
)

// Every generated PHP vhost bounds how long a request waits to *connect* to the
// pool. nginx's default is 60s, and a restarted FPM container comes back on a
// new address the resolver has yet to drop, so one request would sit on the old
// address for the full minute and then answer 504:
//
//	upstream timed out (110: Operation timed out) while connecting to upstream
//
// read/send timeouts are the site's own RequestTimeout and stay as they are:
// this is only about reaching the pool, which on the lerd network is immediate
// or not happening at all.
func TestPHPVhostTemplatesBoundTheConnectTimeout(t *testing.T) {
	for _, name := range []string{"vhost.conf.tmpl", "vhost-ssl.conf.tmpl"} {
		body, err := templateFS.ReadFile("templates/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		src := string(body)
		if !strings.Contains(src, "fastcgi_pass") {
			t.Fatalf("%s no longer passes to fastcgi; this test is watching the wrong file", name)
		}
		if !strings.Contains(src, "fastcgi_connect_timeout") {
			t.Errorf("%s does not set fastcgi_connect_timeout, so a stale upstream costs nginx's 60s default", name)
		}
	}
}
