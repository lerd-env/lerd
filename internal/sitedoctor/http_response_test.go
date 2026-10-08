package sitedoctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// A cold app can take seconds to answer its root, so the probe must overlap the
// container-exec checks instead of queueing behind them. The command check here
// only passes if the probe has already run while it is still waiting.
func TestRun_probesTheSiteAlongsideTheCommandChecks(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}})
	marker := filepath.Join(site.Path, "probed")
	realProbe, realNginx := httpProbe, nginxUp
	httpProbe = func(string) (int, error) { return 200, os.WriteFile(marker, nil, 0o644) }
	nginxUp = func() bool { return true }
	t.Cleanup(func() { httpProbe, nginxUp = realProbe, realNginx })

	fw := &config.Framework{
		Name: "app",
		Doctor: &config.FrameworkDoctor{Checks: []config.DoctorCheck{{
			Name: "waits_for_probe", Type: "command", TimeoutSeconds: 5,
			Command: "for i in $(seq 50); do [ -f probed ] && exit 0; sleep 0.1; done; exit 1",
		}}},
	}
	statuses := map[string]string{}
	for _, c := range Run(context.Background(), site.Path, fw).Checks {
		statuses[c.Name] = c.Status
	}
	if statuses["waits_for_probe"] != StatusOK {
		t.Errorf("command check = %q, want %q: the http probe did not run alongside it", statuses["waits_for_probe"], StatusOK)
	}
	if statuses["http response"] != StatusOK {
		t.Errorf("http response = %q, want %q", statuses["http response"], StatusOK)
	}
}

// The web panel draws each finding as it lands, so every check in the report
// must also reach OnCheck, already labelled the way the report labels it.
func TestRunWith_streamsEveryCheckThroughOnCheck(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}})
	stubProbe(t, 200, nil)
	fw := &config.Framework{
		Name: "app",
		Doctor: &config.FrameworkDoctor{Checks: []config.DoctorCheck{
			{Name: "first_cmd", Type: "command", Command: "true"},
			{Name: "second_cmd", Type: "command", Command: "false"},
		}},
	}
	streamed := map[string]Check{}
	resp := RunWith(context.Background(), site.Path, fw, Options{OnCheck: func(c Check) { streamed[c.Name] = c }})

	if len(streamed) != len(resp.Checks) {
		t.Fatalf("streamed %d checks, report has %d", len(streamed), len(resp.Checks))
	}
	for _, c := range resp.Checks {
		if streamed[c.Name] != c {
			t.Errorf("streamed %+v, report has %+v", streamed[c.Name], c)
		}
	}
}
