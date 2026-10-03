package podman

import (
	"strings"
	"testing"
)

// On Windows the project sits on a 9p share where every stat crosses to the
// host, so FPM stops revalidating and lerd flushes OPcache on change instead.
// Everywhere else the unit must render exactly as before.
func TestFPMArgsOnlyOnWindows(t *testing.T) {
	if got := fpmArgs("windows"); got != " -d opcache.validate_timestamps=0" {
		t.Errorf("fpmArgs(windows) = %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := fpmArgs(goos); got != "" {
			t.Errorf("fpmArgs(%s) = %q, want nothing", goos, got)
		}
	}
}

func TestFPMQuadletExecCarriesTheArgs(t *testing.T) {
	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatal(err)
	}
	want := "Exec=php-fpm -F -R" + fpmArgs(goosForFPM) + "\n"
	if !strings.Contains(content, want) {
		t.Errorf("rendered FPM unit lacks %q", want)
	}
	if strings.Contains(content, "{{") {
		t.Error("a template placeholder was left unrendered")
	}
}
