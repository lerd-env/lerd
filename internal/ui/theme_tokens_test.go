package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A Tailwind class naming a colour token that does not exist renders as
// nothing, so a button silently loses its background instead of failing the
// build. This caught bg-lerd-accent, which was never a token.
func TestSvelteUsesOnlyDefinedLerdColourTokens(t *testing.T) {
	root := filepath.Join("web", "src")
	css, err := os.ReadFile(filepath.Join(root, "app.css"))
	if err != nil {
		t.Fatalf("reading app.css: %v", err)
	}
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`--color-lerd-([a-z0-9-]+)\s*:`).FindAllStringSubmatch(string(css), -1) {
		defined[m[1]] = true
	}
	if len(defined) == 0 {
		t.Fatal("found no --color-lerd-* tokens; the guard would pass vacuously")
	}

	use := regexp.MustCompile(`\b(?:bg|text|border|ring|from|to)-lerd-([a-z0-9-]+)`)
	var scanned int
	var unknown []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".svelte") {
			return err
		}
		if strings.Contains(path, string(filepath.Separator)+"paraglide"+string(filepath.Separator)) {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range use.FindAllStringSubmatch(string(body), -1) {
			scanned++
			if !defined[m[1]] {
				unknown = append(unknown, path+": lerd-"+m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if scanned < 100 {
		t.Fatalf("only %d token usages scanned; the guard is not reaching the components", scanned)
	}
	if len(unknown) > 0 {
		t.Errorf("undefined lerd colour tokens (these render as no colour):\n  %s", strings.Join(unknown, "\n  "))
	}
}
