package webserver

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const nginxPkg = "github.com/geodro/lerd/internal/nginx"

// stillImportsNginx lists the files outside the seam that reach nginx directly.
// Every phase moves callers behind webserver and deletes their lines here; the
// list must end empty, and a new direct import fails the test.
var stillImportsNginx = map[string]bool{
	"cmd/lerd/main.go":                       true,
	"internal/cli/install.go":                true,
	"internal/cli/pause.go":                  true,
	"internal/cli/php_runtime.go":            true,
	"internal/cli/startstop.go":              true,
	"internal/profiler/profiler.go":          true,
	"internal/serviceops/dashboard_vhost.go": true,
	"internal/serviceops/domain.go":          true,
	"internal/siteops/nginxoverride.go":      true,
	"internal/ui/nginx_global.go":            true,
	"internal/ui/remote_control.go":          true,
}

func TestOnlyTheSeamImportsNginx(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") ||
				strings.HasPrefix(rel, "internal/nginx/") || strings.HasPrefix(rel, "internal/webserver/") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				if p, _ := strconv.Unquote(imp.Path.Value); p == nginxPkg {
					found[rel] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for rel := range found {
		if !stillImportsNginx[rel] {
			t.Errorf("%s imports internal/nginx; go through internal/webserver instead", rel)
		}
	}
	for rel := range stillImportsNginx {
		if !found[rel] {
			t.Errorf("%s no longer imports internal/nginx; delete it from stillImportsNginx", rel)
		}
	}
}
