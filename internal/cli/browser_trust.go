package cli

import (
	"fmt"
	"os"
	"strings"
)

// ostreeBootedFn reports whether the host booted from an ostree/atomic image
// (Silverblue, Bazzite, Kinoite, CoreOS), where nss-tools can't be added
// without layering a package and rebooting. Seam for tests.
var ostreeBootedFn = func() bool {
	_, err := os.Stat("/run/ostree-booted")
	return err == nil
}

// browserTrustGuidance returns the message shown when managed DNS is on but
// certutil is missing, so .test HTTPS is trusted by curl, PHP and openssl yet
// browsers warn. The route to browser trust differs on atomic images, which
// can only gain nss-tools by layering it and rebooting.
func browserTrustGuidance(atomic bool) string {
	base := "certutil (nss-tools) not found, so .test certificates are trusted by curl, PHP and openssl but browsers will warn. "
	if atomic {
		return base + "For browser trust run rpm-ostree install nss-tools, reboot, then lerd dns:repair. Or run lerd dns:disable to serve plain http on .localhost."
	}
	return base + "For browser trust install nss-tools (dnf install nss-tools, apt install libnss3-tools, or pacman -S nss) then lerd dns:repair. Or run lerd dns:disable to serve plain http on .localhost."
}

// browserTrustStoreGuidance returns the message shown when certutil is there to
// install the CA and the store does not have it: the browsers reading that store
// warn on .test even though the command-line tools accept it. Naming the stores
// keeps the difference visible between one stale Firefox profile and a machine
// where the CA never reached a browser at all.
func browserTrustStoreGuidance(stores []string) string {
	return fmt.Sprintf("the mkcert root CA is missing from %s, so browsers reading it warn on .test even though curl, PHP and openssl trust it. Run lerd dns:repair to install it there.", strings.Join(stores, ", "))
}
