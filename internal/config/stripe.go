package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// DefaultStripeWebhookPath is the route a Stripe listener forwards events to
// when the project does not configure one. Matches Laravel Cashier's default.
const DefaultStripeWebhookPath = "/stripe/webhook"

// StripeSecretEnvCandidates are the env var names that commonly hold a Stripe
// secret key, in priority order: Laravel/Cashier, the Stripe Node/Nest
// convention, then the generic SDK name. Probed when a project pins no explicit
// secret_env_key, so the listener detects a secret regardless of framework.
var StripeSecretEnvCandidates = []string{"STRIPE_SECRET", "STRIPE_SECRET_KEY", "STRIPE_API_KEY"}

// StripeConfig is the optional per-project Stripe listener config in .lerd.yaml.
// Both fields are optional: empty values fall back to auto-detection and the
// default webhook path, so Laravel projects need no config at all.
type StripeConfig struct {
	// Path is the webhook route the listener forwards events to, e.g.
	// "/webhooks/stripe". Empty uses DefaultStripeWebhookPath.
	Path string `yaml:"path,omitempty"`
	// SecretEnvKey pins which .env key holds the Stripe secret. Empty probes
	// StripeSecretEnvCandidates in order.
	SecretEnvKey string `yaml:"secret_env_key,omitempty"`
}

// ResolveStripeSecret returns the env key name and value of a project's Stripe
// secret. An explicit secret_env_key in .lerd.yaml wins; otherwise the common
// candidate keys are probed in .env. An empty value means none is set.
func ResolveStripeSecret(sitePath string) (envKey, value string) {
	envPath := filepath.Join(sitePath, ".env")
	if proj, err := LoadProjectConfig(sitePath); err == nil && proj.Stripe != nil && proj.Stripe.SecretEnvKey != "" {
		return proj.Stripe.SecretEnvKey, envfile.ReadKey(envPath, proj.Stripe.SecretEnvKey)
	}
	for _, k := range StripeSecretEnvCandidates {
		if v := envfile.ReadKey(envPath, k); v != "" {
			return k, v
		}
	}
	return "", ""
}

// StripeWebhookPath returns the configured webhook route for a project, or
// DefaultStripeWebhookPath when none is set. The stored value is normalised to
// a leading slash so a hand-edited ".lerd.yaml" can't yield a slash-less URL
// like "https://site.teststripe/webhook".
func StripeWebhookPath(sitePath string) string {
	if proj, err := LoadProjectConfig(sitePath); err == nil && proj.Stripe != nil && proj.Stripe.Path != "" {
		return ensureLeadingSlash(proj.Stripe.Path)
	}
	return DefaultStripeWebhookPath
}

func ensureLeadingSlash(p string) string {
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// ValidateStripeWebhookPath normalises a webhook route to a leading slash and
// rejects whitespace, which would otherwise break the systemd ExecStart line
// the path is interpolated into (a space adds an argument, a newline ends the
// directive). An empty path is returned unchanged for callers that leave it
// unset.
func ValidateStripeWebhookPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.ContainsAny(path, " \t\r\n") {
		return "", fmt.Errorf("invalid Stripe webhook path %q: must not contain whitespace", path)
	}
	return ensureLeadingSlash(path), nil
}

// StripeSecretSet reports whether a project has a Stripe secret in .env under
// its configured or any recognised env key.
func StripeSecretSet(sitePath string) bool {
	_, v := ResolveStripeSecret(sitePath)
	return v != ""
}
