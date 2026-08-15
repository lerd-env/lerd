package cli

import (
	"context"
	"net/url"
	"time"

	"github.com/geodro/lerd/internal/origin"
	"github.com/geodro/lerd/internal/podman"
)

// dnsProbeTimeout bounds the lookup. A broken resolver usually hangs rather
// than answering, and doctor must not stall on it.
const dnsProbeTimeout = 8 * time.Second

// dnsProbeContainer is the container the external-resolution probe runs in.
// lerd-nginx is the one that has to be up for anything to be served, so it is
// the container most likely to be available to ask.
const dnsProbeContainer = "lerd-nginx"

// containerDNSProbe is what checkContainerExternalDNS needs, kept as fields so
// the check can be driven without a running container.
type containerDNSProbe struct {
	report      *DoctorReport
	ok          func(label string)
	fail        func(label, msg, hint string)
	warn        func(label, msg string)
	containerUp func(name string) bool
	resolve     func(container, host string) error
	host        string
}

// containerDNSProbeHost is the hostname the probe asks for: the framework
// store's own host, so an install pointed at a self-hosted store is checked
// against the name it actually depends on rather than one lerd never contacts.
func containerDNSProbeHost() string {
	for _, raw := range origin.StoreBaseURLs() {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return ""
}

// checkContainerExternalDNS reports whether a container can resolve an ordinary
// internet hostname. Every other DNS check covers .test resolving back to the
// host, which says nothing about the direction that matters to composer and the
// rest of the in-container tooling: out to the internet.
func checkContainerExternalDNS(p containerDNSProbe) {
	if p.host == "" {
		return
	}
	if !p.containerUp(dnsProbeContainer) {
		p.warn("container internet DNS", "skipped — "+dnsProbeContainer+" not running (start lerd first)")
		return
	}
	if err := p.resolve(dnsProbeContainer, p.host); err != nil {
		p.fail("container internet DNS",
			"containers cannot resolve "+p.host+" (composer and every other in-container download will fail)",
			"the lerd network's forwarders are stale or unreachable; inspect with: podman network inspect lerd")
		p.report.fixLast(autoFix(fixContainerDNS, "", "re-point the lerd network at the current resolvers"))
		return
	}
	p.ok("container internet DNS (" + p.host + ")")
}

// resolveInContainer asks a running container to resolve host, which is the
// same lookup composer makes before it downloads anything.
func resolveInContainer(container, host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dnsProbeTimeout)
	defer cancel()
	return podman.CmdContext(ctx, "exec", container, "getent", "hosts", host).Run()
}
