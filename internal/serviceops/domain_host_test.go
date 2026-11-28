package serviceops

import (
	"fmt"
	"strings"
	"testing"
)

// A host nginx cannot resolve lerd-<service> on the podman network, so a service
// domain proxies to the loopback port the container publishes for that port.
func TestApplyServiceDomain_OnHostProxiesToThePublishedPort(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)
	prev := domainOnHostFn
	t.Cleanup(func() { domainOnHostFn = prev })
	domainOnHostFn = func() bool { return true }

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	container := serviceDomainPort("rustfs")
	want := ""
	for _, m := range HostPortMappings("rustfs") {
		parts := strings.Split(m, ":")
		if strings.SplitN(parts[len(parts)-1], "/", 2)[0] == fmt.Sprint(container) {
			want = fmt.Sprint("|127.0.0.1|", parts[len(parts)-2], "|")
			break
		}
	}
	if want == "" {
		t.Fatalf("rustfs publishes no host port for %d: %v", container, HostPortMappings("rustfs"))
	}
	last := rec.vhosts[len(rec.vhosts)-1]
	if !strings.Contains(last, want) {
		t.Errorf("vhost = %q, want the loopback port (%q)", last, want)
	}
}
