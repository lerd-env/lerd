package sitetpl

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestExpandCommands(t *testing.T) {
	in := []config.FrameworkCommand{
		{Name: "install", Command: "bin/magento setup:install --base-url={{scheme}}://{{domain}}/ --db-name={{site}}"},
		{Name: "migrate", Command: "php artisan migrate"},
	}
	out := ExpandCommands(in, Ctx{Site: "shop", Domain: "shop.test", Scheme: "http"})

	want := "bin/magento setup:install --base-url=http://shop.test/ --db-name=shop"
	if out[0].Command != want {
		t.Errorf("got  %q\nwant %q", out[0].Command, want)
	}
	if out[1].Command != "php artisan migrate" {
		t.Errorf("untemplated command changed: %q", out[1].Command)
	}
	// Other fields survive, and the caller's slice is untouched.
	if out[0].Name != "install" {
		t.Errorf("Name lost: %q", out[0].Name)
	}
	if in[0].Command == out[0].Command {
		t.Error("ExpandCommands mutated the input slice")
	}
}

func TestExpandCommandsEmptySlice(t *testing.T) {
	if got := ExpandCommands(nil, Ctx{Site: "x"}); len(got) != 0 {
		t.Fatalf("got %d commands", len(got))
	}
}

// An unregistered path (a git worktree is not a registered site) still yields a
// database handle, but no domain or scheme. Those placeholders are then left
// verbatim by Apply rather than collapsing to "://".
func TestForPathUnregisteredHasNoDomain(t *testing.T) {
	ctx := ForPath("/tmp/definitely-not-a-registered-lerd-site")
	if ctx.Domain != "" || ctx.Scheme != "" {
		t.Fatalf("unregistered path resolved a domain/scheme: %+v", ctx)
	}
	if ctx.Site == "" {
		t.Fatal("expected a fallback site handle")
	}
	got := Apply("--url={{scheme}}://{{domain}}/ --db={{site}}", ctx)
	if !strings.Contains(got, "{{scheme}}://{{domain}}") {
		t.Fatalf("placeholders were collapsed: %q", got)
	}
}

func TestForSiteNil(t *testing.T) {
	if got := (ForSite(nil)); got != (Ctx{}) {
		t.Fatalf("got %+v", got)
	}
}

func TestForSiteSchemeFollowsSecured(t *testing.T) {
	insecure := ForSite(&config.Site{Name: "shop", Path: "/tmp/nonexistent-shop", Domains: []string{"shop.test"}})
	if insecure.Scheme != "http" {
		t.Errorf("got scheme %q, want http", insecure.Scheme)
	}
	secure := ForSite(&config.Site{Name: "shop", Path: "/tmp/nonexistent-shop", Domains: []string{"shop.test"}, Secured: true})
	if secure.Scheme != "https" {
		t.Errorf("got scheme %q, want https", secure.Scheme)
	}
	if secure.Domain != "shop.test" {
		t.Errorf("got domain %q", secure.Domain)
	}
}
