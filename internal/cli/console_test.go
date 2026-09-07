package cli

import (
	"strings"
	"testing"
)

// Framework console commands build their own exec, so the provider's declared
// variables have to be forwarded there too, by name.
func TestConsoleCmdArgsForwardsPassthroughEnv(t *testing.T) {
	t.Setenv("LERD_PASSTHROUGH_ENV", "APP_KEY,STRIPE_*")
	t.Setenv("APP_KEY", "base64:abc")
	t.Setenv("STRIPE_SECRET", "sk_test")

	args := consoleCmdArgs("/srv/app", "lerd-php85-fpm", "artisan", false, []string{"migrate"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--env APP_KEY --env STRIPE_SECRET") {
		t.Errorf("consoleCmdArgs() = %q, missing the forwarded variables", joined)
	}
	for _, a := range args {
		if a == "sk_test" || strings.Contains(a, "APP_KEY=") {
			t.Errorf("consoleCmdArgs() leaked a value into argv: %q", a)
		}
	}
}
