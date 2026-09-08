package cli

import (
	"strings"
	"testing"
)

// Run outside a site, restart used to fall through to the directory name and
// report it as a missing site. The command has to say what it is for instead,
// including that it is not how lerd is restarted.
func TestRestartTargetNameOutsideASite(t *testing.T) {
	_, err := restartTargetName(nil, func(string) bool { return false })
	if err == nil {
		t.Fatal("expected an error outside a site")
	}
	for _, want := range []string{"inside a site", "lerd restart <site>", "not lerd itself"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message should mention %q, got: %v", want, err)
		}
	}
}

// A named site is taken as given: naming one is how the command is pointed at a
// site from anywhere, and the run itself reports a name that does not exist.
func TestRestartTargetNameTakesTheNamedSite(t *testing.T) {
	got, err := restartTargetName([]string{"shop"}, func(string) bool { return false })
	if err != nil || got != "shop" {
		t.Errorf("named site = %q, %v; want shop", got, err)
	}
}
