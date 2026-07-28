package config

import "testing"

// The registry serialises through its own on-disk struct, so a field added to
// Site alone is silently dropped on save.
func TestSiteDevServerPortSurvivesRoundTrip(t *testing.T) {
	in := Site{Name: "myapp", Path: "/srv/myapp", DevServerPort: 5180}
	out := in.toYAML().toSite()
	if out.DevServerPort != 5180 {
		t.Fatalf("DevServerPort = %d after round trip, want 5180", out.DevServerPort)
	}
}
