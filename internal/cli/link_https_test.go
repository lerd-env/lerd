package cli

import "testing"

func TestShouldSecureOnLink(t *testing.T) {
	cases := []struct {
		name                                 string
		projSecured, siteSecured, dnsManaged bool
		want                                 bool
	}{
		{"a project asking for HTTPS gets it", true, false, true, true},
		{"already secured needs nothing", true, true, true, false},
		{"a localhost install cannot issue a certificate", true, false, false, false},
		// The bug: an absent .lerd.yaml reads as secured:false, and treating
		// that as intent dropped a secured site to HTTP on every re-link.
		{"a project with no opinion never turns HTTPS off", false, true, true, false},
		{"no opinion and no HTTPS stays put", false, false, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldSecureOnLink(c.projSecured, c.siteSecured, c.dnsManaged); got != c.want {
				t.Errorf("shouldSecureOnLink(%v, %v, %v) = %v, want %v",
					c.projSecured, c.siteSecured, c.dnsManaged, got, c.want)
			}
		})
	}
}
