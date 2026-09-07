package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// terminalInputPatterns are the operations that take bytes from the terminal or
// change its mode. Every incident in this area came from one of them appearing
// somewhere new: a progress view reading keys while an installer question was
// waiting for the same keystrokes, a raw mode left on under a prompt, a
// duplicated descriptor whose reader could not be stopped.
var terminalInputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`term\.MakeRaw\(`),
	regexp.MustCompile(`term\.ReadPassword\(`),
	regexp.MustCompile(`bufio\.New(Reader|Scanner)\(os\.Stdin`),
	regexp.MustCompile(`fmt\.Fscan[a-z]*\(os\.Stdin`),
	regexp.MustCompile(`os\.Stdin\.Read`),
	regexp.MustCompile(`Dup\(int\(os\.Stdin`),
	regexp.MustCompile(`startHotkeys\(`),
	regexp.MustCompile(`"/dev/tty"`),
}

// terminalInputOwners are the files allowed to do it today. The list exists to
// shrink: #1708 collapses it to a single package that owns the terminal for the
// whole process. Until then, a new entry means a new thing competing for the
// user's keystrokes, so add one only after checking it cannot run while
// something else is reading.
var terminalInputOwners = map[string]bool{
	"internal/cli/build_ui.go":             true,
	"internal/cli/fpm_ensure.go":           true,
	"internal/cli/framework.go":            true,
	"internal/cli/install.go":              true,
	"internal/cli/hotkeys.go":              true,
	"internal/cli/machine_reset_darwin.go": true,
	"internal/cli/remote_control.go":       true,
	"internal/cli/setup.go":                true,
	"internal/cli/startstop.go":            true,
	"internal/mcp/server.go":               true,
}

// TestTerminalInputStaysWithItsOwners fails when a file that is not a known
// owner starts reading the terminal or switching its mode. Passing here is not
// proof the new use is safe, only that nobody added one without deciding to.
func TestTerminalInputStaysWithItsOwners(t *testing.T) {
	root := repoRoot(t)
	var found []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil || terminalInputOwners[filepath.ToSlash(rel)] {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, re := range terminalInputPatterns {
				if re.Match(body) {
					found = append(found, filepath.ToSlash(rel)+" matches "+re.String())
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		t.Fatalf("new code reads the terminal outside the files that own it:\n  %s\n\nIf it really must, add the file to terminalInputOwners and say why it cannot run while something else is reading.", strings.Join(found, "\n  "))
	}
}

// repoRoot walks up from this package to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test package")
		}
		dir = parent
	}
}
