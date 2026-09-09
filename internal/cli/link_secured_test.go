package cli

import "testing"

// A project that records secured: true is describing how it is served, so a
// link has to put the site back on HTTPS. Without this a reinstall re-registered
// every site as plain HTTP and https refused outright until `lerd secure` was
// run by hand, which is the one thing the .lerd.yaml was there to prevent.
func TestLinkShouldRestoreHTTPS(t *testing.T) {
	cases := []struct {
		name                                 string
		projSecured, siteSecured, dnsManaged bool
		want                                 bool
	}{
		{"project says secured and the site is not", true, false, true, true},
		{"site already secured", true, true, true, false},
		{"project says nothing", false, false, true, false},
		{"external DNS has no certificates to issue", true, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkShouldRestoreHTTPS(tc.projSecured, tc.siteSecured, tc.dnsManaged); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
