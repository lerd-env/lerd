package sitedoctor

import (
	"strings"
	"testing"
)

// A native site is served by a PHP-FPM on the host. If that listener is not
// answering, every request 502s with nothing in the site's own logs to explain
// it, so the doctor says so plainly.
func TestNativeListenerCheck(t *testing.T) {
	cases := []struct {
		name       string
		listening  bool
		wantStatus string
	}{
		{"up", true, StatusOK},
		{"down", false, StatusFail},
	}
	for _, c := range cases {
		got := nativeListenerCheck("8.4", 9484, func(int) bool { return c.listening })
		if got.Status != c.wantStatus {
			t.Errorf("%s: status = %q, want %q", c.name, got.Status, c.wantStatus)
		}
		if c.wantStatus == StatusFail && !strings.Contains(got.Detail, "9484") {
			t.Errorf("a failing check should name the port, got %q", got.Detail)
		}
	}
}

// The native build carries a fixed extension set. A project requiring one it
// does not have would fail at runtime with a message that says nothing about
// the runtime it is on, so the drift is reported up front.
func TestNativeExtensionCheck(t *testing.T) {
	have := []string{"intl", "redis", "pdo_mysql"}

	ok := nativeExtensionCheck([]string{"intl", "redis"}, have)
	if ok.Status != StatusOK {
		t.Errorf("all requirements present should be OK, got %q (%s)", ok.Status, ok.Detail)
	}

	missing := nativeExtensionCheck([]string{"intl", "imagick", "mongodb"}, have)
	if missing.Status != StatusWarn {
		t.Errorf("missing extensions should warn, got %q", missing.Status)
	}
	for _, want := range []string{"imagick", "mongodb"} {
		if !strings.Contains(missing.Detail, want) {
			t.Errorf("detail should name %q, got %q", want, missing.Detail)
		}
	}
	if strings.Contains(missing.Detail, "intl") {
		t.Errorf("detail should not name a satisfied extension, got %q", missing.Detail)
	}
}
