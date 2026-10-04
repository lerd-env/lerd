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

// TestSharedCodeDoesNotBranchOnGOOS fails when a file compiled for more than
// one OS compares or switches on runtime.GOOS.
func TestSharedCodeDoesNotBranchOnGOOS(t *testing.T) {
	var problems []string
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
				problems = append(problems, fmt.Sprintf("%s: %d runtime.GOOS branches; move them into a per-OS file or platform.Current", filepath.ToSlash(rel), n))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
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
