package sitedoctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubProbe points the check at a canned answer and restores the real one.
func stubProbe(t *testing.T, code int, err error) *string {
	t.Helper()
	var asked string
	realProbe, realNginx := httpProbe, nginxUp
	httpProbe = func(url string) (int, error) { asked = url; return code, err }
	nginxUp = func() bool { return true }
	t.Cleanup(func() { httpProbe, nginxUp = realProbe, realNginx })
	return &asked
}

// The whole point of the check: every other one reads configuration, and
// configuration can be perfect while the app answers 500 to every request.
func TestCheckHTTPResponse_failsOnAServerError(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Secured: true})
	stubProbe(t, 500, nil)

	c, ok := checkHTTPResponse(site.Path)
	if !ok {
		t.Fatal("no check produced for a registered site")
	}
	if c.Status != StatusFail {
		t.Errorf("status = %q, want %q for a 500", c.Status, StatusFail)
	}
	if !strings.Contains(c.Detail, "500") {
		t.Errorf("detail = %q, want the status code in it", c.Detail)
	}
}

// A skeleton with no routes answers 404 on / by design, and nothing here can
// tell that apart from a real one, so a 4xx must not read as a broken site.
func TestCheckHTTPResponse_acceptsA404(t *testing.T) {
	site := registerSite(t, config.Site{Name: "shop", Domains: []string{"shop.test"}, Secured: true})
	stubProbe(t, 404, nil)

	c, _ := checkHTTPResponse(site.Path)
	if c.Status != StatusOK {
		t.Errorf("status = %q, want %q for a 404", c.Status, StatusOK)
	}
}

func TestCheckHTTPResponse_failsWhenNothingAnswers(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}})
	stubProbe(t, 0, errors.New("connection refused"))

	c, _ := checkHTTPResponse(site.Path)
	if c.Status != StatusFail {
		t.Errorf("status = %q, want %q when the site does not answer", c.Status, StatusFail)
	}
}

// With nginx down every site fails identically and none of it is the site's
// doing, so the check stands aside rather than blaming the app.
func TestCheckHTTPResponse_standsAsideWhenNginxIsDown(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}})
	realProbe, realNginx := httpProbe, nginxUp
	probed := false
	httpProbe = func(string) (int, error) { probed = true; return 200, nil }
	nginxUp = func() bool { return false }
	t.Cleanup(func() { httpProbe, nginxUp = realProbe, realNginx })

	c, _ := checkHTTPResponse(site.Path)
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want %q with nginx down", c.Status, StatusWarn)
	}
	if probed {
		t.Error("the site was probed even though nginx is not running")
	}
}

// The scheme follows the site's TLS state, and a moved nginx port has to be in
// the URL or the probe knocks on a door nothing listens at.
func TestSiteRootURL_followsSchemeAndPort(t *testing.T) {
	secured := config.Site{Name: "a", Domains: []string{"a.test"}, Secured: true}
	if got := siteRootURL(secured); got != "https://a.test" {
		t.Errorf("secured url = %q, want https://a.test", got)
	}
	plain := config.Site{Name: "b", Domains: []string{"b.test"}}
	if got := siteRootURL(plain); got != "http://b.test" {
		t.Errorf("plain url = %q, want http://b.test", got)
	}
}
