package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// nginx is whitespace insensitive, so a value only needs a `;` and braces to
// close the directive it lands in and open a server block of its own. A
// project's .lerd.yaml supplies the domains, the public dir and the path, so
// none of them may carry those characters into a generated vhost.
const nginxPayload = `x; } server { listen 80; server_name pwn.test; root /etc; autoindex on; }`

func confsUnder(t *testing.T, dir string) string {
	t.Helper()
	var all strings.Builder
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".conf") {
			return nil
		}
		b, _ := os.ReadFile(p)
		all.WriteString(p + "\n" + string(b))
		return nil
	})
	return all.String()
}

func TestGenerateVhostRefusesInjectedValues(t *testing.T) {
	cases := []struct {
		field string
		site  config.Site
	}{
		{"domain", config.Site{Name: "probe", Domains: []string{"clean.test", nginxPayload}, PHPVersion: "8.4"}},
		{"primary domain", config.Site{Name: "probe", Domains: []string{nginxPayload}, PHPVersion: "8.4"}},
		// The paths are quoted by Root(), so `;` is literal there; a newline is
		// the one thing the quoting cannot contain.
		{"public dir", config.Site{Name: "probe", Domains: []string{"clean.test"}, PublicDir: "public\n    autoindex on;\n    #", PHPVersion: "8.4"}},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", tmp)
			t.Setenv("XDG_DATA_HOME", tmp)
			site := tc.site
			site.Path = t.TempDir()

			if err := GenerateVhost(site, "8.4"); err == nil {
				t.Errorf("%s carrying nginx syntax was accepted", tc.field)
			}
			if got := confsUnder(t, tmp); strings.Contains(got, "autoindex on") {
				t.Errorf("%s: a vhost carrying the injected block was written:\n%s", tc.field, got)
			}
		})
	}
}

// A worktree vhost takes its domain from a git branch, and git allows `;`, `{`
// and `}` in a refname.
func TestGenerateWorktreeVhostRefusesInjectedBranch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	err := GenerateWorktreeVhost("clean.test", t.TempDir(), "8.4", "probe", nginxPayload)
	if err == nil {
		t.Error("a branch carrying nginx syntax was accepted")
	}
	if got := confsUnder(t, tmp); strings.Contains(got, "autoindex on") {
		t.Errorf("a worktree vhost carrying the injected block was written:\n%s", got)
	}
}

// A directory whose name legitimately carries nginx punctuation still serves:
// the path is quoted, so it never had to be refused.
func TestGenerateVhostAcceptsAPathWithNginxPunctuation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := filepath.Join(t.TempDir(), "my#project;v2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	site := config.Site{
		Name: "shop", Domains: []string{"shop.test"},
		Path: dir, PublicDir: "public", PHPVersion: "8.4",
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Errorf("a path with legal punctuation was refused: %v", err)
	}
}

// The ordinary shape still renders.
func TestGenerateVhostAcceptsNormalValues(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	site := config.Site{
		Name: "shop", Domains: []string{"shop.test", "www-shop.test"},
		Path: t.TempDir(), PublicDir: "public", PHPVersion: "8.4",
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	got := confsUnder(t, tmp)
	if !strings.Contains(got, "server_name shop.test") {
		t.Errorf("vhost lost its server_name:\n%s", got)
	}
}
