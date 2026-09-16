package ui

import "testing"

// The dashboard keys "is there a container for this site" off the runtime field,
// and its helper already returns none for "native". Nothing ever set it: native
// is an install-wide mode rather than a per-site one, so a plain FPM site kept
// reporting the shared FPM container and the UI offered to restart, shell into
// and tail something that does not exist.
func TestReportedRuntime(t *testing.T) {
	cases := []struct {
		name                    string
		siteRuntime             string
		native                  bool
		containerPort, hostPort int
		want                    string
	}{
		{"plain fpm under native", "", true, 0, 0, "native"},
		{"plain fpm under container", "", false, 0, 0, ""},
		{"frankenphp keeps its own", "frankenphp", true, 0, 0, "frankenphp"},
		{"custom fpm keeps its own", "fpm-custom", true, 0, 0, "fpm-custom"},
		{"custom container is unaffected", "", true, 8080, 0, ""},
		{"host proxy is unaffected", "", true, 0, 3000, ""},
	}
	for _, c := range cases {
		if got := reportedRuntime(c.siteRuntime, c.native, c.containerPort, c.hostPort); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
