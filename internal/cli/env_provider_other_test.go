//go:build !darwin

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

func providerSite(t *testing.T, provider string) config.Site {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("env_provider is Linux-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "run"))
	site := config.Site{Name: "app", Path: t.TempDir()}
	if err := config.SaveProjectConfig(site.Path, &config.ProjectConfig{EnvProvider: provider}); err != nil {
		t.Fatalf("write .lerd.yaml: %v", err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}
	return site
}

func TestRefreshProvidedEnv_WritesOwnerOnlyFile(t *testing.T) {
	provider := "printf 'SECRET=s3cret\\n'"
	site := providerSite(t, provider)
	if err := config.ApproveSiteCommand(site.Name, provider); err != nil {
		t.Fatalf("ApproveSiteCommand: %v", err)
	}
	if err := refreshProvidedEnv(site, false); err != nil {
		t.Fatalf("refreshProvidedEnv: %v", err)
	}
	file := config.ProvidedEnvFile(site.Name)
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("provided env not written: %v", err)
	}
	root, _ := filepath.EvalSymlinks(site.Path)
	if string(body) != "#lerd-root="+root+"\nSECRET=s3cret\n" {
		t.Errorf("body = %q", body)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestRefreshProvidedEnv_UnapprovedIsRefused(t *testing.T) {
	site := providerSite(t, "printf 'SECRET=x\\n'")
	if err := refreshProvidedEnv(site, false); err == nil {
		t.Fatal("an unapproved provider must not run")
	}
	if _, err := os.Stat(config.ProvidedEnvFile(site.Name)); !os.IsNotExist(err) {
		t.Errorf("no file may be written for an unapproved provider: %v", err)
	}
}

func TestRefreshProvidedEnv_FailingProviderKeepsPreviousFile(t *testing.T) {
	site := providerSite(t, "exit 3")
	if err := config.ApproveSiteCommand(site.Name, "exit 3"); err != nil {
		t.Fatal(err)
	}
	file := config.ProvidedEnvFile(site.Name)
	if err := writeProvidedEnv(file, []byte("OLD=1\n")); err != nil {
		t.Fatal(err)
	}
	if err := refreshProvidedEnv(site, false); err == nil {
		t.Fatal("expected the provider's failure to surface")
	}
	if body, _ := os.ReadFile(file); string(body) != "OLD=1\n" {
		t.Errorf("a failed refresh must leave the last good file, got %q", body)
	}
}

func TestRefreshProvidedEnv_RemovesStaleFileWithoutProvider(t *testing.T) {
	site := providerSite(t, "")
	file := config.ProvidedEnvFile(site.Name)
	if err := writeProvidedEnv(file, []byte("OLD=1\n")); err != nil {
		t.Fatal(err)
	}
	if err := refreshProvidedEnv(site, false); err != nil {
		t.Fatalf("refreshProvidedEnv: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("stale provided env should be removed: %v", err)
	}
}

func TestRefreshProvidedEnv_YesApprovesAndRemembers(t *testing.T) {
	provider := "printf 'SECRET=x\\n'"
	site := providerSite(t, provider)
	if err := refreshProvidedEnv(site, true); err != nil {
		t.Fatalf("--yes should approve the provider: %v", err)
	}
	if err := refreshProvidedEnv(site, false); err != nil {
		t.Errorf("the approval should be remembered for later runs: %v", err)
	}
}

// Unlinking a site drops its env_provider secrets through the real
// dropProvidedEnv the CLI wires into siteops, so nothing is left for another
// site's PHP to find.
func TestTeardownSite_RemovesProvidedEnv(t *testing.T) {
	site := providerSite(t, "")
	file := config.ProvidedEnvFile(site.Name)
	if err := writeProvidedEnv(file, []byte("SECRET=x\n")); err != nil {
		t.Fatal(err)
	}
	stop := siteops.StopSiteWorkers
	siteops.StopSiteWorkers = nil
	t.Cleanup(func() { siteops.StopSiteWorkers = stop })
	site.Domains = []string{"app.test"}
	siteops.TeardownSite(&site, nil)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("provided env should be removed on unlink: %v", err)
	}
}
