//go:build windows

package cli

import (
	"io"
	"os"

	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/imagepull"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
)

const dnsUnit = "lerd-dns"

// dnsServiceContent is the unit for lerd's built-in DNS answerer. The exe path
// is wrapped in plain quotes, not %q: a Windows profile path can hold spaces,
// and %q would double every backslash.
func dnsServiceContent(exe string) string {
	return "[Unit]\nDescription=Lerd DNS\n\n[Service]\nExecStart=\"" + exe + "\" dns-serve\nRestart=always\n"
}

// installDNSService registers the built-in DNS answerer as the lerd-dns unit.
// gvproxy cannot forward UDP into a container, so DNS runs on the host, the
// same reason macOS runs dnsmasq natively.
func installDNSService(_ io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	removeDNSContainerIfRunning()
	_, err = services.Mgr.WriteServiceUnitIfChanged(dnsUnit, dnsServiceContent(exe))
	return err
}

// needsDNSServiceInstall is true until the unit has been written.
func needsDNSServiceInstall() bool {
	return len(services.Mgr.ListServiceUnits(dnsUnit)) == 0
}

func ensureDNSImageForStart() {}

// pullDNSImages is a no-op: DNS runs on the host and needs no image.
func pullDNSImages() []BuildJob { return nil }

func dnsImagePlan() imagepull.Plan { return nil }

func isDNSContainerUnit() bool { return false }

func writeDNSUnit(_ io.Writer) error { return installDNSService(io.Discard) }

func ensureDNSServiceUpdated(w io.Writer) error {
	if needsDNSServiceInstall() {
		feedback.LineOn(w, "Installing the built-in DNS service…")
		return installDNSService(w)
	}
	return nil
}

// removeDNSContainerIfRunning clears a container left by an earlier layout,
// which would otherwise hold the port the host answerer needs.
func removeDNSContainerIfRunning() {
	podman.Cmd("stop", dnsUnit).Run()     //nolint:errcheck
	podman.Cmd("rm", "-f", dnsUnit).Run() //nolint:errcheck
}

func nativeDNSRestart() error { return services.Mgr.Restart(dnsUnit) }

// teardownDNS stops lerd-dns and removes its unit and the NRPT rule, so
// disabling DNS never leaves the resolver pointing at a server that is gone.
func teardownDNS() {
	_ = services.Mgr.Stop(dnsUnit)
	_ = services.Mgr.RemoveServiceUnit(dnsUnit)
	_ = services.Mgr.RemoveContainerUnit(dnsUnit)
	dns.Teardown()
}
