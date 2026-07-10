package nginx

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/geodro/lerd/internal/config"
)

func renderVhostForTest(t *testing.T, name string, data VhostData) string {
	t.Helper()
	raw, err := GetTemplate(name)
	if err != nil {
		t.Fatalf("GetTemplate(%s): %v", name, err)
	}
	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return buf.String()
}

func TestIndentBlock(t *testing.T) {
	got := indentBlock("a {\n  b;\n\n}", "    ")
	want := "    a {\n      b;\n\n    }"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExpandNginxSnippet(t *testing.T) {
	got, err := expandNginxSnippet(
		"root {{root}};\nalias {{public}};\nfastcgi_pass {{fpm}}:9000;",
		"/home/u/shop", "pub", "lerd-php84-fpm",
	)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	for _, want := range []string{"root /home/u/shop;", "alias /home/u/shop/pub;", "fastcgi_pass lerd-php84-fpm:9000;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("unexpanded placeholder in:\n%s", got)
	}
}

// `{{roots}}` has balanced braces, so ValidateNginxSnippet accepts it. If it
// reached nginx verbatim it would break the config for every site on the box.
func TestExpandNginxSnippetRejectsUnknownPlaceholder(t *testing.T) {
	snippet := "root {{roots}};"
	if err := config.ValidateNginxSnippet(snippet); err != nil {
		t.Fatalf("typo has balanced braces, should validate: %v", err)
	}
	if _, err := expandNginxSnippet(snippet, "/p", "pub", "fpm"); err == nil {
		t.Error("unknown placeholder was accepted")
	}
}

// A "." public_dir means the project root is the document root.
func TestExpandNginxSnippetDotPublicDir(t *testing.T) {
	got, err := expandNginxSnippet("root {{public}};", "/home/u/wp", ".", "fpm")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if got != "root /home/u/wp;" {
		t.Fatalf("got %q", got)
	}
}

// Brace balance cannot contain an interpolated value: `}` plus `server {` still
// balances. So hostile values must be rejected at substitution time instead.
func TestExpandNginxSnippetRejectsHostileValues(t *testing.T) {
	snippet := "location /a/ {\n    root {{root}};\n}"
	if err := config.ValidateNginxSnippet(snippet); err != nil {
		t.Fatalf("snippet should be balanced: %v", err)
	}

	escape := "/home/u/x}\nserver { listen 81;"
	if _, err := expandNginxSnippet(snippet, escape, "pub", "fpm"); err == nil {
		t.Error("a path that closes the block and opens a server was accepted")
	}
	// It would have passed a post-expansion brace check, which is the whole point.
	balanced := strings.NewReplacer("{{root}}", escape).Replace(snippet)
	if err := config.ValidateNginxSnippet(balanced); err != nil {
		t.Logf("post-expansion check happened to catch it: %v", err)
	}

	for _, bad := range []string{"/p;deny all", "/p{", "/p}", "/p#c", "/p\nx", "/p\x00"} {
		if _, err := expandNginxSnippet(snippet, bad, "pub", "fpm"); err == nil {
			t.Errorf("path %q accepted", bad)
		}
	}
	if _, err := expandNginxSnippet(snippet, "/p", "pub", "fpm;deny all"); err == nil {
		t.Error("hostile fpm container name accepted")
	}
	if _, err := expandNginxSnippet(snippet, "/p", "pub}x{", "fpm"); err == nil {
		t.Error("hostile public dir accepted")
	}
}

// The framework block must land inside the server block and BEFORE the generic
// `location /` and `location ~ \.php$`, since nginx picks the first matching
// regex location in declaration order.
func TestVhostRendersFrameworkNginxBeforeGenericLocations(t *testing.T) {
	data := VhostData{
		Domain:         "shop.test",
		ServerNames:    "shop.test",
		Path:           "/home/u/shop",
		PublicDir:      "pub",
		FPMContainer:   "lerd-php84-fpm",
		RequestTimeout: 60,
		FrameworkNginx: "    location ~* ^/setup($|/) {\n        root /home/u/shop;\n    }",
	}
	out := renderVhostForTest(t, "vhost.conf.tmpl", data)

	iSetup := strings.Index(out, "^/setup($|/)")
	iRoot := strings.Index(out, "root /home/u/shop/pub;")
	iSlash := strings.Index(out, "location / {")
	iPHP := strings.Index(out, `location ~ \.php$`)
	iClose := strings.LastIndex(out, "}")

	if iSetup < 0 {
		t.Fatalf("framework block missing:\n%s", out)
	}
	if !(iRoot < iSetup && iSetup < iSlash && iSetup < iPHP && iSetup < iClose) {
		t.Fatalf("bad ordering root=%d setup=%d slash=%d php=%d close=%d\n%s", iRoot, iSetup, iSlash, iPHP, iClose, out)
	}
}

func TestVhostOmitsFrameworkNginxWhenAbsent(t *testing.T) {
	for _, tmpl := range []string{"vhost.conf.tmpl", "vhost-ssl.conf.tmpl"} {
		data := VhostData{
			Domain: "a.test", ServerNames: "a.test", Path: "/p", PublicDir: "public",
			CertDomain: "a.test", FPMContainer: "f", RequestTimeout: 60,
		}
		out := renderVhostForTest(t, tmpl, data)
		if strings.Contains(out, "FrameworkNginx") {
			t.Fatalf("%s leaked field name", tmpl)
		}
		if !strings.Contains(out, "location / {") {
			t.Fatalf("%s missing generic location:\n%s", tmpl, out)
		}
	}
}

func TestSSLVhostRendersFrameworkNginx(t *testing.T) {
	data := VhostData{
		Domain: "shop.test", ServerNames: "shop.test", Path: "/home/u/shop", PublicDir: "pub",
		CertDomain: "shop.test", FPMContainer: "f", RequestTimeout: 60,
		FrameworkNginx: "    location /media/ {\n        try_files $uri /get.php;\n    }",
	}
	out := renderVhostForTest(t, "vhost-ssl.conf.tmpl", data)
	iMedia := strings.Index(out, "location /media/ {")
	iSlash := strings.Index(out, "location / {")
	if iMedia < 0 || iMedia > iSlash {
		t.Fatalf("media=%d slash=%d\n%s", iMedia, iSlash, out)
	}
}
