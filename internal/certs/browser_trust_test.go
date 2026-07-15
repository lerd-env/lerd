package certs

import "testing"

func TestBrowserTrustAvailable(t *testing.T) {
	orig := certutilLookup
	defer func() { certutilLookup = orig }()

	certutilLookup = func() bool { return true }
	if !BrowserTrustAvailable() {
		t.Fatal("want available when certutil is present")
	}

	certutilLookup = func() bool { return false }
	if BrowserTrustAvailable() {
		t.Fatal("want unavailable when certutil is missing")
	}
}
