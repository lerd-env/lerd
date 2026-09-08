package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func corsVhostBody(t *testing.T, cors bool) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := GenerateServiceProxyVhost("rustfs.test", "lerd-rustfs", 9000, true, cors); err != nil {
		t.Fatalf("GenerateServiceProxyVhost: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(config.NginxConfD(), "rustfs.test.conf"))
	if err != nil {
		t.Fatalf("reading vhost: %v", err)
	}
	return string(body)
}

// A presigned upload is sent by the browser, which asks before it sends and
// gives up unless the answer names the origin the page came from.
func TestServiceVhostCORS_AnswersThePreflight(t *testing.T) {
	got := corsVhostBody(t, true)
	for _, want := range []string{
		`if ($request_method = OPTIONS)`,
		"return 204;",
		"add_header Access-Control-Allow-Methods",
		`add_header Access-Control-Expose-Headers "ETag" always;`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vhost missing %q:\n%s", want, got)
		}
	}
}

// The origin is reflected, never wildcarded: "*" would let any page the user
// happens to have open read a local service over JS.
func TestServiceVhostCORS_ReflectsOnlyLerdOrigins(t *testing.T) {
	got := corsVhostBody(t, true)
	if !strings.Contains(got, `if ($http_origin ~ "^https?://[^/]+\.test(:[0-9]+)?$")`) {
		t.Errorf("origin is not matched against the TLD lerd serves:\n%s", got)
	}
	if !strings.Contains(got, "set $cors $http_origin;") {
		t.Errorf("origin is not reflected:\n%s", got)
	}
	if strings.Contains(got, "Access-Control-Allow-Origin *") ||
		strings.Contains(got, `Access-Control-Allow-Origin "*"`) {
		t.Errorf("origin was wildcarded:\n%s", got)
	}
}

// An object store that sends CORS headers of its own would otherwise leave two
// of each on the response, and a browser rejects that outright rather than
// picking one, so ours only land after the upstream's are dropped.
func TestServiceVhostCORS_DropsTheUpstreamHeadersFirst(t *testing.T) {
	got := corsVhostBody(t, true)
	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Expose-Headers",
		"Access-Control-Max-Age",
	} {
		if !strings.Contains(got, "proxy_hide_header "+header+";") {
			t.Errorf("upstream %s is not hidden, so the browser can see two:\n%s", header, got)
		}
	}
}

// The response differs by the origin that asked for it, so a cache must not
// hand one origin's copy to another.
func TestServiceVhostCORS_VariesOnOrigin(t *testing.T) {
	if got := corsVhostBody(t, true); !strings.Contains(got, "add_header Vary Origin always;") {
		t.Errorf("response does not vary on Origin:\n%s", got)
	}
}

// Safari does not accept "*" for the allowed headers, so the preflight answers
// with the headers that were actually asked for.
func TestServiceVhostCORS_ReflectsRequestedHeaders(t *testing.T) {
	got := corsVhostBody(t, true)
	if !strings.Contains(got, "add_header Access-Control-Allow-Headers $http_access_control_request_headers always;") {
		t.Errorf("requested headers are not reflected:\n%s", got)
	}
}

// A service nobody calls from a page is not quietly made readable by one.
func TestServiceVhostCORS_AbsentUnlessAskedFor(t *testing.T) {
	if got := corsVhostBody(t, false); strings.Contains(got, "Access-Control") {
		t.Errorf("CORS rendered without being asked for:\n%s", got)
	}
}

func TestCORSOriginPattern(t *testing.T) {
	cases := []struct{ tld, want string }{
		{"test", `^https?://[^/]+\.test(:[0-9]+)?$`},
		{"localhost", `^https?://[^/]+\.localhost(:[0-9]+)?$`},
		// A dotted TLD has its dots escaped rather than left matching any character.
		{"dev.local", `^https?://[^/]+\.dev\.local(:[0-9]+)?$`},
		// The TLD comes from user config; one that was never a TLD must not
		// reach the regex and widen what it matches.
		{`bad" { deny all; }`, `^https?://[^/]+\.test(:[0-9]+)?$`},
		{"", `^https?://[^/]+\.test(:[0-9]+)?$`},
	}
	for _, c := range cases {
		if got := corsOriginPattern(c.tld); got != c.want {
			t.Errorf("corsOriginPattern(%q) = %q, want %q", c.tld, got, c.want)
		}
	}
}

// The origin regex reaches nginx inside a quoted directive, so it must not be
// able to end one, whatever the configured TLD is.
func TestCORSOriginPatternCannotEndADirective(t *testing.T) {
	for _, tld := range []string{"test", "dev.local", `x"; }`, "a;b"} {
		if i := strings.IndexAny(corsOriginPattern(tld), nginxValueForbidden); i >= 0 {
			t.Errorf("pattern for TLD %q carries %q", tld, string(corsOriginPattern(tld)[i]))
		}
	}
}
