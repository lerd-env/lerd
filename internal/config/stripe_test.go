package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestResolveStripeSecret_ProbesCandidates(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		wantKey string
		wantVal string
	}{
		{"laravel", "STRIPE_SECRET=sk_laravel\n", "STRIPE_SECRET", "sk_laravel"},
		{"node", "STRIPE_SECRET_KEY=sk_node\n", "STRIPE_SECRET_KEY", "sk_node"},
		{"sdk", "STRIPE_API_KEY=sk_sdk\n", "STRIPE_API_KEY", "sk_sdk"},
		{"none", "OTHER=x\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, ".env"), tc.env)
			key, val := ResolveStripeSecret(dir)
			if key != tc.wantKey || val != tc.wantVal {
				t.Errorf("ResolveStripeSecret = (%q, %q), want (%q, %q)", key, val, tc.wantKey, tc.wantVal)
			}
			if got := StripeSecretSet(dir); got != (tc.wantVal != "") {
				t.Errorf("StripeSecretSet = %v, want %v", got, tc.wantVal != "")
			}
		})
	}
}

func TestResolveStripeSecret_ConfiguredKeyWins(t *testing.T) {
	dir := t.TempDir()
	// Both a candidate and a non-candidate key are present; the pinned one wins
	// even though STRIPE_SECRET would otherwise be picked first.
	writeFile(t, filepath.Join(dir, ".env"), "STRIPE_SECRET=sk_default\nMY_STRIPE=sk_custom\n")
	writeFile(t, filepath.Join(dir, ".lerd.yaml"), "stripe:\n  secret_env_key: MY_STRIPE\n")
	key, val := ResolveStripeSecret(dir)
	if key != "MY_STRIPE" || val != "sk_custom" {
		t.Errorf("ResolveStripeSecret = (%q, %q), want (MY_STRIPE, sk_custom)", key, val)
	}
}

func TestStripeWebhookPath_DefaultAndConfigured(t *testing.T) {
	dir := t.TempDir()
	if got := StripeWebhookPath(dir); got != DefaultStripeWebhookPath {
		t.Errorf("StripeWebhookPath with no config = %q, want %q", got, DefaultStripeWebhookPath)
	}
	writeFile(t, filepath.Join(dir, ".lerd.yaml"), "stripe:\n  path: /webhooks/stripe\n")
	if got := StripeWebhookPath(dir); got != "/webhooks/stripe" {
		t.Errorf("StripeWebhookPath = %q, want /webhooks/stripe", got)
	}
}

func TestSetProjectStripe_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	if err := SetProjectStripe(dir, "/webhooks/stripe", "STRIPE_SECRET_KEY"); err != nil {
		t.Fatalf("SetProjectStripe: %v", err)
	}
	proj, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if proj.Stripe == nil || proj.Stripe.Path != "/webhooks/stripe" || proj.Stripe.SecretEnvKey != "STRIPE_SECRET_KEY" {
		t.Fatalf("persisted stripe config = %+v, want path /webhooks/stripe and key STRIPE_SECRET_KEY", proj.Stripe)
	}
	// A second call with one empty arg must not clobber the existing field.
	if err := SetProjectStripe(dir, "/new/path", ""); err != nil {
		t.Fatalf("SetProjectStripe second: %v", err)
	}
	proj2, _ := LoadProjectConfig(dir)
	if proj2.Stripe.Path != "/new/path" || proj2.Stripe.SecretEnvKey != "STRIPE_SECRET_KEY" {
		t.Errorf("after partial update = %+v, want path /new/path and key preserved", proj2.Stripe)
	}
}

func TestValidateStripeWebhookPath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"/stripe/webhook", "/stripe/webhook", false},
		{"webhooks/stripe", "/webhooks/stripe", false},
		{"", "", false},
		{"/has space", "", true},
		{"/has\nnewline", "", true},
	}
	for _, tc := range cases {
		got, err := ValidateStripeWebhookPath(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ValidateStripeWebhookPath(%q) expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ValidateStripeWebhookPath(%q) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
		}
	}
}

func TestSetProjectStripe_BothEmptyIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := SetProjectStripe(dir, "", ""); err != nil {
		t.Fatalf("SetProjectStripe(empty): %v", err)
	}
	// No .lerd.yaml should be created and no empty stripe block persisted.
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err == nil {
		t.Errorf("SetProjectStripe with both args empty must not create .lerd.yaml")
	}
}

func TestSetProjectStripe_NormalizesAndRejectsPath(t *testing.T) {
	dir := t.TempDir()
	if err := SetProjectStripe(dir, "webhooks/stripe", ""); err != nil {
		t.Fatalf("SetProjectStripe: %v", err)
	}
	if got := StripeWebhookPath(dir); got != "/webhooks/stripe" {
		t.Errorf("stored path = %q, want /webhooks/stripe (leading slash added)", got)
	}
	if err := SetProjectStripe(dir, "/bad path", ""); err == nil {
		t.Errorf("SetProjectStripe must reject a path containing whitespace")
	}
}

func TestStripeWebhookPath_NormalizesHandEdited(t *testing.T) {
	dir := t.TempDir()
	// A hand-edited .lerd.yaml without a leading slash must still resolve to a
	// well-formed route so the forward URL keeps its separator.
	writeFile(t, filepath.Join(dir, ".lerd.yaml"), "stripe:\n  path: stripe/webhook\n")
	if got := StripeWebhookPath(dir); got != "/stripe/webhook" {
		t.Errorf("StripeWebhookPath = %q, want /stripe/webhook", got)
	}
}

func TestProjectConfig_StripeNotEmpty(t *testing.T) {
	c := &ProjectConfig{Stripe: &StripeConfig{Path: "/x"}}
	if c.IsEmpty() {
		t.Errorf("ProjectConfig with a Stripe block must not be IsEmpty")
	}
}
