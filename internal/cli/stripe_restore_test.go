package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

type enabledServiceMgr struct{ fakeServiceMgr }

func (enabledServiceMgr) IsEnabled(string) bool { return true }

// An enabled listener written before the image was pinned has to be rewritten
// on start, or it keeps running the floating image an upgrade was meant to fix.
func TestRestoreStripeWorkerRewritesEnabledListener(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sitePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sitePath, ".env"), []byte("STRIPE_SECRET=sk_test_x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: sitePath}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatal(err)
	}
	fake := &enabledServiceMgr{}
	swapMgr(t, fake)

	restoreStripeWorker(site)

	if !slices.Contains(fake.calls, "write:lerd-stripe-shop") {
		t.Fatalf("enabled listener unit was not rewritten, calls: %v", fake.calls)
	}
}
