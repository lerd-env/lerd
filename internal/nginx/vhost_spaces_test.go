package nginx

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A project under a path with a space renders a `root` directive with three
// arguments, which nginx refuses to parse. It rejects the whole config, so one
// such site takes every other site down with it (#893).
func TestVhostQuotesDocumentRootWithSpaces(t *testing.T) {
	for _, name := range []string{"vhost.conf.tmpl", "vhost-ssl.conf.tmpl"} {
		out := renderVhostForTest(t, name, VhostData{
			Domain:         "spatnik.test",
			ServerNames:    "spatnik.test *.spatnik.test",
			Path:           "/media/tim/DriveX/My Laravel CMS/Spatnik",
			PublicDir:      "public",
			FPMContainer:   "lerd-php85-fpm",
			CertDomain:     "spatnik.test",
			RequestTimeout: 60,
		})
		want := `root "/media/tim/DriveX/My Laravel CMS/Spatnik/public";`
		if !strings.Contains(out, want) {
			t.Errorf("%s: want %q in:\n%s", name, want, out)
		}
	}
}

// Backslash-escaping the spaces would pass `nginx -t` and then 404 every
// request: nginx keeps the backslash in the value and realpath() fails on it.
func TestVhostRootIsQuotedNotBackslashEscaped(t *testing.T) {
	out := renderVhostForTest(t, "vhost.conf.tmpl", VhostData{
		Domain:       "spatnik.test",
		Path:         "/media/My Laravel CMS/Spatnik",
		PublicDir:    "public",
		FPMContainer: "lerd-php85-fpm",
	})
	if strings.Contains(out, `My\ Laravel`) {
		t.Errorf("root backslash-escaped, which nginx does not resolve:\n%s", out)
	}
}

// A framework snippet interpolates the paths mid-token, as Magento's does with
// {{public}}/static/, and a quoted token cannot be glued to a bare one. The
// paths reach the snippet as variables, which nginx resolves after tokenizing.
func TestExpandNginxSnippetUsesPathVariables(t *testing.T) {
	got, err := expandNginxSnippet(
		"root {{root}};\nalias {{public}}/static/;\nfastcgi_pass {{fpm}}:9000;",
		"/media/tim/My Laravel CMS/shop", "pub", "lerd-php84-fpm",
	)
	if err != nil {
		t.Fatalf("expandNginxSnippet: %v", err)
	}
	want := "set $lerd_root \"/media/tim/My Laravel CMS/shop\";\n" +
		"set $lerd_public \"/media/tim/My Laravel CMS/shop/pub\";\n\n" +
		"root ${lerd_root};\n" +
		"alias ${lerd_public}/static/;\n" +
		"fastcgi_pass lerd-php84-fpm:9000;"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A snippet that names no path gets no variables: nothing would read them.
func TestExpandNginxSnippetWithoutPathsDeclaresNoVariables(t *testing.T) {
	got, err := expandNginxSnippet("fastcgi_pass {{fpm}}:9000;", "/home/u/shop", "public", "lerd-php84-fpm")
	if err != nil {
		t.Fatalf("expandNginxSnippet: %v", err)
	}
	if strings.Contains(got, "set $lerd_") {
		t.Errorf("declared unused path variables:\n%s", got)
	}
}

// The paused and idle-waking landing pages are served from lerd's own data dir,
// which sits under $HOME and inherits a space from the user's home directory.
func TestLandingVhostQuotesRoot(t *testing.T) {
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}}
	out := landingVhostConf(site, "/home/My User/.local/share/lerd/paused", "shop.test.html")
	if want := `root "/home/My User/.local/share/lerd/paused";`; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

func TestNginxQuote(t *testing.T) {
	cases := map[string]string{
		"/home/u/shop":          `"/home/u/shop"`,
		"/media/My Laravel CMS": `"/media/My Laravel CMS"`,
		`/media/back\slash`:     `"/media/back\\slash"`,
		`/media/qu"ote`:         `"/media/qu\"ote"`,
	}
	for in, want := range cases {
		if got := nginxQuote(in); got != want {
			t.Errorf("nginxQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
