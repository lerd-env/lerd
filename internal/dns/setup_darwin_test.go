//go:build darwin

package dns

import (
	"os"
	"path/filepath"
	"sort"
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
