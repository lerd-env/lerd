//go:build darwin

package dns

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// staleResolverFiles must return exactly the resolver files lerd wrote (matched
// by content, across TLDs) and leave a user's or another tool's resolver files
// and subdirectories alone.
func TestStaleResolverFiles(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, body []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("test", resolverContent)                           // canonical lerd TLD
	write("app", resolverContent)                            // preserved custom lerd TLD
	write("test-trailing", append(resolverContent, '\n'))    // lerd content + stray newline
	write("corp", []byte("nameserver 10.0.0.53\nport 53\n")) // foreign, must survive
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	got := staleResolverFiles(dir)
	sort.Strings(got)

	want := []string{
		filepath.Join(dir, "app"),
		filepath.Join(dir, "test"),
		filepath.Join(dir, "test-trailing"),
	}
	if len(got) != len(want) {
		t.Fatalf("staleResolverFiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("staleResolverFiles = %v, want %v", got, want)
		}
	}
}

// A missing resolver directory is not an error: Teardown on a host that never
// configured DNS must be a no-op.
func TestStaleResolverFilesMissingDir(t *testing.T) {
	if got := staleResolverFiles(filepath.Join(t.TempDir(), "does-not-exist")); got != nil {
		t.Fatalf("expected nil for a missing dir, got %v", got)
	}
}

// codeOnly strips // line comments so a source scan asserts against code, not
// prose: a comment mentioning the token it forbids must not trip the check.
func codeOnly(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// ruleLines drops the sudoers header comment (which legitimately contains an
// ellipsis like /var/folders/.../lerd-sudo-*) so a traversal scan sees only the
// actual NOPASSWD rules.
func ruleLines(grant string) string {
	var b strings.Builder
	for _, line := range strings.Split(grant, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// On macOS the TLD lands in /etc/resolver/<tld> and, through the sudoers grant,
// in a NOPASSWD `tee` target. A raw config value could traverse out of
// /etc/resolver or inject a sudoers line, so both must go through ConfiguredTLD,
// which rejects anything that is not a DNS label. This pins the wiring: a
// reintroduced raw cfg.DNS.TLD read for either sink would fail here.
func TestDarwinResolver_routesTheTLDThroughTheValidator(t *testing.T) {
	src, err := os.ReadFile("setup_darwin.go")
	if err != nil {
		t.Fatalf("reading setup_darwin.go: %v", err)
	}
	code := codeOnly(string(src))
	if strings.Contains(code, "cfg.DNS.TLD") {
		t.Error("setup_darwin.go reads cfg.DNS.TLD raw; the resolver path and sudoers target must come from ConfiguredTLD")
	}
	if !strings.Contains(code, "ConfiguredTLD()") {
		t.Error("setup_darwin.go must derive the TLD from ConfiguredTLD")
	}
}

// A traversal or injection payload must never produce a resolver path outside
// /etc/resolver nor a sudoers target that escapes it.
func TestDarwinSudoers_tldCannotEscapeTheResolverDir(t *testing.T) {
	for _, bad := range []string{"../../etc/cron.d/x", "a/b", "x\nEXTRA"} {
		tld := DefaultTLD
		if tldPattern.MatchString(bad) {
			tld = bad
		}
		rules := ruleLines(renderDarwinSudoers("alice", tld))
		if strings.Contains(rules, "..") || strings.Contains(rules, "/etc/resolver/a/") ||
			strings.Contains(rules, "EXTRA") {
			t.Errorf("payload %q reached the sudoers grant", bad)
		}
	}
}
