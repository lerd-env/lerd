package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFrameworkNginxParsesFromYAML(t *testing.T) {
	src := `
name: magento
version: "2"
public_dir: pub
nginx:
  snippet: |
    location /static/ {
        try_files $uri $uri/ /static.php?$args;
    }
`
	var fw Framework
	if err := yaml.Unmarshal([]byte(src), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.Nginx == nil {
		t.Fatal("nginx block not parsed")
	}
	if !strings.Contains(fw.Nginx.Snippet, "try_files $uri $uri/ /static.php?$args;") {
		t.Fatalf("snippet not preserved: %q", fw.Nginx.Snippet)
	}
}

func TestValidateNginxSnippet(t *testing.T) {
	cases := []struct {
		name    string
		snippet string
		wantErr bool
	}{
		{"empty", "", false},
		{"balanced", "location /a/ {\n    try_files $uri /a.php;\n}", false},
		{"nested balanced", "location /a/ {\n  location /b/ {\n    deny all;\n  }\n}", false},
		{"brace in comment ignored", "# a } brace\nlocation /a/ {\n    deny all;\n}", false},
		// The containment property: a snippet must not be able to close the
		// enclosing `server {` block and start declaring its own servers.
		{"escapes server block", "}\nserver { listen 80; }\nlocation /a/ {", true},
		{"trailing unclosed", "location /a/ {", true},
		{"extra closing brace", "location /a/ {\n}\n}", true},
		{"nul byte", "location /a/ {\x00}", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNginxSnippet(tc.snippet)
			if tc.wantErr && err == nil {
				t.Fatalf("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil, got %v", err)
			}
		})
	}
}

// A .lerd.yaml is untrusted input. It must not be able to inject nginx config
// into the site's server block via an embedded framework_def, the same way it
// cannot inject host-executing commands or doctor checks.
func TestSanitizeProjectFrameworkDefStripsNginx(t *testing.T) {
	def := &Framework{
		Name:  "evil",
		Nginx: &FrameworkNginx{Snippet: "location /x/ { deny all; }"},
	}
	safe := SanitizeProjectFrameworkDef(def)
	if safe.Nginx != nil {
		t.Fatalf("nginx survived sanitize: %+v", safe.Nginx)
	}
	if def.Nginx == nil {
		t.Fatal("sanitize mutated the caller's definition")
	}
}
