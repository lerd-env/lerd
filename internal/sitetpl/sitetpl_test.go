package sitetpl

import (
	"strings"
	"testing"
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
