package serviceops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
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

// Every port the unit publishes counts, not only the primary: a moved secondary
// port and an extra port both reach the host nginx at their own loopback port.
func TestServiceUpstream_OnHostFollowsEveryPublishedPort(t *testing.T) {
	domainTestConfig(t)
	prev := domainOnHostFn
	t.Cleanup(func() { domainOnHostFn = prev })
	domainOnHostFn = func() bool { return true }

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	sc := cfg.Services["mailpit"]
	sc.PublishedPorts = map[int]int{8025: 18026}
	sc.ExtraPorts = []string{"19000:9000"}
	cfg.Services["mailpit"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	for container, want := range map[int]int{8025: 18026, 9000: 19000} {
		host, port, err := serviceUpstream("mailpit", container)
		if err != nil || host != "127.0.0.1" || port != want {
			t.Errorf("serviceUpstream(mailpit, %d) = %s:%d, %v; want 127.0.0.1:%d", container, host, port, err, want)
		}
	}
}
