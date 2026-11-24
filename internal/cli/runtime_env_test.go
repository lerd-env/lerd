package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

// stubRerunSiteEnv records the sites lerd env was rerun in, without spawning it.
func stubRerunSiteEnv(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	orig := rerunSiteEnv
	rerunSiteEnv = func(dir string) { ran = append(ran, dir) }
	t.Cleanup(func() { rerunSiteEnv = orig })
	return &ran
}

// In container mode FPM and FrankenPHP both reach services by container name,
// so switching between them has nothing to rewrite in .env.
func TestRuntimeSwitchLeavesEnvAloneInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ran := stubRerunSiteEnv(t)

	site := &config.Site{Name: "shop", Path: "/p/shop"}
	wasNative := site.IsNative()
	site.Runtime = "frankenphp"
	if siteops.SyncEnvAfterRuntimeSwitch(site, wasNative) || len(*ran) != 0 {
		t.Errorf("container mode must not rerun lerd env, ran in %v", *ran)
	}
}
