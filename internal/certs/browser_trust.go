package certs

import "os/exec"

// certutilLookup reports whether certutil is resolvable on PATH. Seam for tests.
var certutilLookup = func() bool {
	_, err := exec.LookPath("certutil")
	return err == nil
}

// BrowserTrustAvailable reports whether mkcert can install the root CA into the
// browser NSS trust stores. That needs certutil, shipped in nss-tools. Without
// it mkcert still writes the CA to the system trust store, so curl, PHP and
// openssl trust .test, but Firefox and Chrome are skipped and warn on HTTPS.
func BrowserTrustAvailable() bool {
	return certutilLookup()
}
