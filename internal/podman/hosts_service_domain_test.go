package podman

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A presigned URL carries its host in the signature, so the app container has
// to reach the service by the same name the browser will use. Without this the
// name resolves only on the host and the app cannot sign for it at all.
func TestRenderContainerHosts_ResolvesServiceDomainsToNginx(t *testing.T) {
	reg := &config.SiteRegistry{Sites: []config.Site{{Name: "shop", Domains: []string{"shop.test"}}}}
	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3", map[string]string{"rustfs": "rustfs.test"})

	if !strings.Contains(got, "10.89.0.3 rustfs.test\n") {
		t.Errorf("service domain missing from the container hosts file:\n%s", got)
	}
	if !strings.Contains(got, "10.89.0.3 shop.test\n") {
		t.Errorf("site domain missing from the container hosts file:\n%s", got)
	}
}

func TestRenderContainerHosts_NoServiceDomainsAddsNothing(t *testing.T) {
	reg := &config.SiteRegistry{Sites: []config.Site{{Name: "shop", Domains: []string{"shop.test"}}}}
	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3", nil)
	if strings.Count(got, "10.89.0.3") != 1 {
		t.Errorf("expected only the site entry, got:\n%s", got)
	}
}

// Two runs of the same configuration have to produce the same file, or the
// change detection that decides whether to rewrite it fires on map order alone.
func TestRenderContainerHosts_ServiceDomainsAreOrdered(t *testing.T) {
	reg := &config.SiteRegistry{}
	domains := map[string]string{"rustfs": "rustfs.test", "mailpit": "mail.test"}
	first := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3", domains)
	for range 20 {
		if got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3", domains); got != first {
			t.Fatalf("output varies between runs:\n%s\n---\n%s", first, got)
		}
	}
}
