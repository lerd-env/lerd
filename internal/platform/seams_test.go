package platform

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// osBranchAllowlist counts the runtime.GOOS branches each shared file still
// carries. Entries only ever shrink: moving a branch behind a per-OS file means
// lowering its count here, and a file with none left drops off the list.
var osBranchAllowlist = map[string]int{
	"internal/cli/bug_report.go":      3,
	"internal/cli/doctor.go":          1,
	"internal/cli/hostproxy.go":       2,
	"internal/cli/install.go":         2,
	"internal/cli/update.go":          2,
	"internal/config/paths.go":        8,
	"internal/dns/diagnose.go":        4,
	"internal/editor/editor.go":       1,
	"internal/git/copy.go":            2,
	"internal/hostbin/hostbin.go":     1,
	"internal/node/bun.go":            1,
	"internal/node/mise.go":           1,
	"internal/node/nvm.go":            1,
	"internal/node/system.go":         1,
	"internal/podman/netnsroute.go":   1,
	"internal/podman/network.go":      1,
	"internal/podman/upgradeheal.go":  1,
	"internal/systemd/networkwait.go": 1,
	"internal/tray/menu.go":           1,
	"internal/tui/sitetabs.go":        1,
	"internal/ui/openfolder.go":       1,
	"internal/ui/server.go":           4,
	"internal/ui/terminal_default.go": 2,
	"internal/watcher/dns.go":         2,
}

// TestSharedCodeDoesNotBranchOnGOOS fails when a file compiled for more than
// one OS compares or switches on runtime.GOOS beyond its allowlisted count.
func TestSharedCodeDoesNotBranchOnGOOS(t *testing.T) {
	found := map[string]int{}
	for _, dir := range []string{"../../cmd", "../../internal"} {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSharedGoFile(t, path) {
				return nil
			}
			if n := countOSBranches(t, path, nil); n > 0 {
				rel, _ := filepath.Rel("../..", path)
				found[filepath.ToSlash(rel)] = n
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	var problems []string
	for file, n := range found {
		if allowed := osBranchAllowlist[file]; n > allowed {
			problems = append(problems, fmt.Sprintf("%s: %d runtime.GOOS branches, %d allowed; move the new one into a per-OS file", file, n, allowed))
		}
	}
	for file, allowed := range osBranchAllowlist {
		if n := found[file]; n < allowed {
			problems = append(problems, fmt.Sprintf("%s: %d runtime.GOOS branches, allowlist still says %d; lower it", file, n, allowed))
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

// isSharedGoFile reports whether a non-test Go file builds for two or more of
// the OSes lerd ships on, judged by its name suffix and build constraint.
func isSharedGoFile(t *testing.T, path string) bool {
	t.Helper()
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	builds := 0
	for _, goos := range []string{"linux", "darwin", "windows"} {
		ctx := build.Default
		ctx.GOOS = goos
		ok, err := ctx.MatchFile(filepath.Dir(path), filepath.Base(path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if ok {
			builds++
		}
	}
	return builds > 1
}

// countOSBranches counts `runtime.GOOS ==`, `!=` and `switch runtime.GOOS` in
// a file, or in src when it is given. Reading GOOS as a value is not counted.
func countOSBranches(t *testing.T, path string, src any) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.BinaryExpr:
			if (x.Op == token.EQL || x.Op == token.NEQ) && (isGOOS(x.X) || isGOOS(x.Y)) {
				n++
			}
		case *ast.SwitchStmt:
			if isGOOS(x.Tag) {
				n++
			}
		}
		return true
	})
	return n
}

func isGOOS(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "GOOS" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "runtime"
}

func TestCountOSBranches(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"equality":        {`if runtime.GOOS == "darwin" {}`, 1},
		"inequality":      {`if "linux" != runtime.GOOS {}`, 1},
		"switch":          {`switch runtime.GOOS { case "linux": }`, 1},
		"passed as data":  {`_ = exeName("lerd", runtime.GOOS)`, 0},
		"shown to a user": {`println("platform:", runtime.GOOS)`, 0},
		"other selector":  {`if build.GOOS == "darwin" {}`, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := "package p\nfunc f() {\n" + c.body + "\n}\n"
			if got := countOSBranches(t, "x.go", src); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
