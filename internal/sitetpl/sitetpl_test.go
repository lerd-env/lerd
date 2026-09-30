package sitetpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestApplyReplacesSiteHandles(t *testing.T) {
	ctx := Ctx{Site: "my_shop", Bucket: "my-shop", Domain: "my-shop.test", Scheme: "https"}
	got := Apply("--base-url={{scheme}}://{{domain}}/ --db-name={{site}} --test-db={{site_testing}} --bucket={{bucket}}", ctx)
	want := "--base-url=https://my-shop.test/ --db-name=my_shop --test-db=my_shop_testing --bucket=my-shop"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// An empty field must leave its placeholder alone rather than substituting "",
// so a half-built context cannot silently produce `--base-url=://`.
func TestApplyLeavesUnknownAndEmptyAlone(t *testing.T) {
	got := Apply("{{domain}} {{scheme}} {{bucket}} {{nope}}", Ctx{Site: "s"})
	for _, want := range []string{"{{domain}}", "{{scheme}}", "{{bucket}}", "{{nope}}"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was substituted away: %q", want, got)
		}
	}
}

func TestApplyNoPlaceholdersIsIdentity(t *testing.T) {
	in := "php artisan migrate --force"
	if got := Apply(in, Ctx{Site: "x", Domain: "d", Scheme: "http"}); got != in {
		t.Fatalf("got %q, want unchanged", got)
	}
}

// {{site}} is always safe to expand: an empty Site would produce a bare flag,
// so it is only replaced when set.
func TestApplyEmptySiteLeavesPlaceholder(t *testing.T) {
	if got := Apply("--db-name={{site}}", Ctx{}); got != "--db-name={{site}}" {
		t.Fatalf("got %q", got)
	}
}

// The database a command template names is the site's own, whichever spelling
// of a symlinked home the project path arrives in.
func TestDBNameFromTheOtherHomeSpelling(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(root, "var-home", "u", "app")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "var-home"), filepath.Join(root, "home")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: site}); err != nil {
		t.Fatal(err)
	}

	if got := DBName(filepath.Join(root, "home", "u", "app")); got != "shop" {
		t.Errorf("DBName(linked spelling) = %q, want shop", got)
	}
}
