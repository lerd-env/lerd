package desktopnotify

import "testing"

func TestClickURLs(t *testing.T) {
	cases := []struct{ route, app, dash string }{
		{"#system", "lerd://open/#system", "http://127.0.0.1:7073/#system"},
		{"/sites/foo", "lerd://open/sites/foo", "http://127.0.0.1:7073/sites/foo"},
		{"", "lerd://open/", "http://127.0.0.1:7073/"},
	}
	for _, tc := range cases {
		if got := appSchemeURL(tc.route); got != tc.app {
			t.Errorf("appSchemeURL(%q)=%q, want %q", tc.route, got, tc.app)
		}
		if got := dashboardURL(tc.route); got != tc.dash {
			t.Errorf("dashboardURL(%q)=%q, want %q", tc.route, got, tc.dash)
		}
	}
}

func TestUrgencyFromString(t *testing.T) {
	cases := map[string]Urgency{
		"":         UrgencyNormal,
		"normal":   UrgencyNormal,
		"low":      UrgencyLow,
		"critical": UrgencyCritical,
		"high":     UrgencyCritical,
		"weird":    UrgencyNormal,
	}
	for in, want := range cases {
		if got := UrgencyFromString(in); got != want {
			t.Errorf("UrgencyFromString(%q)=%d, want %d", in, got, want)
		}
	}
}
