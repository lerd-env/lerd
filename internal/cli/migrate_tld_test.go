package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

func TestSitesWithTLD_PicksOnlyMatchingSuffix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	for _, s := range []config.Site{
		{Name: "alpha", Path: filepath.Join(tmp, "alpha"), Domains: []string{"alpha.test"}},
		{Name: "beta", Path: filepath.Join(tmp, "beta"), Domains: []string{"beta.localhost"}},
		{Name: "gamma", Path: filepath.Join(tmp, "gamma"), Domains: []string{"gamma.test", "gamma-alt.test"}},
		{Name: "delta", Path: filepath.Join(tmp, "delta"), Domains: []string{"delta.example.com"}},
	} {
		if err := config.AddSite(s); err != nil {
			t.Fatalf("AddSite %s: %v", s.Name, err)
		}
	}

	got := sitesWithTLD("test")
	want := []string{"alpha", "gamma"}
	if !slices.Equal(got, want) {
		t.Errorf("sitesWithTLD(test) = %v, want %v", got, want)
	}
}

func TestMigrateSiteTLD_RewritesDomainsAndEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatalf("mkdir NginxConfD: %v", err)
	}

	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatalf("mkdir site: %v", err)
	}
	envPath := filepath.Join(siteDir, ".env")
	if err := os.WriteFile(envPath, []byte("APP_URL=http://alpha.test\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	// The project committed HTTPS intent; disabling DNS must leave it intact.
	if err := config.SaveProjectConfig(siteDir, &config.ProjectConfig{Secured: true}); err != nil {
		t.Fatalf("save .lerd.yaml: %v", err)
	}

	staleVhost := filepath.Join(config.NginxConfD(), "alpha.test.conf")
	if err := os.WriteFile(staleVhost, []byte("server {}\n"), 0644); err != nil {
		t.Fatalf("write vhost: %v", err)
	}

	certsDir := filepath.Join(config.CertsDir(), "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatalf("mkdir certs: %v", err)
	}
	staleCrt := filepath.Join(certsDir, "alpha.test.crt")
	staleKey := filepath.Join(certsDir, "alpha.test.key")
	for _, p := range []string{staleCrt, staleKey} {
		if err := os.WriteFile(p, []byte("dummy\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	if err := config.AddSite(config.Site{
		Name:    "alpha",
		Path:    siteDir,
		Domains: []string{"alpha.test"},
		Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	changed := migrateSiteTLD("test", "localhost", true)
	if !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("changed = %v, want [alpha]", changed)
	}

	site, err := config.FindSite("alpha")
	if err != nil {
		t.Fatalf("FindSite: %v", err)
	}
	if got := site.PrimaryDomain(); got != "alpha.localhost" {
		t.Errorf("primary domain = %q, want alpha.localhost", got)
	}
	if site.Secured {
		t.Errorf("Secured should be false after forceUnsecure")
	}

	proj, err := config.LoadProjectConfig(siteDir)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if !proj.Secured {
		t.Errorf(".lerd.yaml secured intent should survive DNS disable; got false")
	}

	envBytes, _ := os.ReadFile(envPath)
	if want := "APP_URL=http://alpha.localhost"; string(envBytes) == "" || !contains(envBytes, want) {
		t.Errorf(".env not updated; got %q, want substring %q", envBytes, want)
	}

	if _, err := os.Stat(staleVhost); !os.IsNotExist(err) {
		t.Errorf("stale vhost should have been removed; stat err = %v", err)
	}
	if _, err := os.Stat(staleCrt); !os.IsNotExist(err) {
		t.Errorf("stale .crt should have been removed; stat err = %v", err)
	}
	if _, err := os.Stat(staleKey); !os.IsNotExist(err) {
		t.Errorf("stale .key should have been removed; stat err = %v", err)
	}
}

func TestMigrateWorktreeVhosts_RewritesConfsAndEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatalf("mkdir confD: %v", err)
	}
	wtPath := filepath.Join(tmp, "alpha-wt")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}
	envPath := filepath.Join(wtPath, ".env")
	if err := os.WriteFile(envPath, []byte("APP_URL=http://feat-x.alpha.test\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	staleConf := filepath.Join(config.NginxConfD(), "feat-x.alpha.test.conf")
	if err := os.WriteFile(staleConf, []byte("server {}\n"), 0644); err != nil {
		t.Fatalf("write stale conf: %v", err)
	}

	worktrees := []gitpkg.Worktree{
		{Name: "wt", Branch: "feat-x", Path: wtPath, Domain: "feat-x.alpha.test"},
	}
	migrateWorktreeVhosts(worktrees, "alpha.localhost", "8.4", "alpha", false, nil)

	if _, err := os.Stat(staleConf); !os.IsNotExist(err) {
		t.Errorf("stale worktree conf should be gone; stat err = %v", err)
	}
	freshConf := filepath.Join(config.NginxConfD(), "feat-x.alpha.localhost.conf")
	if _, err := os.Stat(freshConf); err != nil {
		t.Errorf("fresh worktree conf missing: %v", err)
	}

	envBytes, _ := os.ReadFile(envPath)
	if !contains(envBytes, "APP_URL=http://feat-x.alpha.localhost") {
		t.Errorf(".env not updated; got %q", envBytes)
	}
}

// A host-proxy worktree checkout usually has no .lerd.yaml of its own, so the
// migration must mirror the parent's proxy config (passed in) rather than
// loading config from the worktree path, or the new vhost never gets written.
func TestMigrateWorktreeVhosts_HostProxyUsesParentProxy(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatalf("mkdir confD: %v", err)
	}
	wtPath := filepath.Join(tmp, "alpha-wt")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("APP_URL=http://feat-x.alpha.test\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	worktrees := []gitpkg.Worktree{
		{Name: "wt", Branch: "feat-x", Path: wtPath, Domain: "feat-x.alpha.test"},
	}
	proxy := &config.ProxyConfig{Command: "npm run dev", Port: 5173}
	migrateWorktreeVhosts(worktrees, "alpha.localhost", "", "alpha", false, proxy)

	freshConf := filepath.Join(config.NginxConfD(), "feat-x.alpha.localhost.conf")
	if _, err := os.Stat(freshConf); err != nil {
		t.Errorf("host-proxy worktree vhost missing without a worktree .lerd.yaml: %v", err)
	}
	envBytes, _ := os.ReadFile(filepath.Join(wtPath, ".env"))
	if !contains(envBytes, "APP_URL=http://feat-x.alpha.localhost") {
		t.Errorf(".env not updated; got %q", envBytes)
	}
}

// TestMigrateSiteTLD_ReissuesCertForSecuredSiteWithWorktree pins the fix
// for the regression where migrating a secured site's TLD regenerated the
// worktree vhosts but never reissued the parent cert. SSL handshakes to
// branch.<newPrimary> would fail because the cert SANs still carried the
// old TLD's wildcard. The migration must produce a cert at the NEW primary
// path that covers the renamed worktree subdomain.
func TestMigrateSiteTLD_ReissuesCertForSecuredSiteWithWorktree(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Fake mkcert that writes its SAN list into the cert/key files so the
	// test can assert the new TLD's wildcard SAN was passed.
	fakeMkcert := `#!/bin/sh
CRT=""
KEY=""
SANS=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -cert-file) shift; CRT="$1" ;;
    -key-file) shift; KEY="$1" ;;
    *) SANS="$SANS $1" ;;
  esac
  shift
done
echo "$SANS" > "$CRT"
echo "FAKE-KEY" > "$KEY"
`
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate a worktree via .git/worktrees/<entry>/ — DetectWorktrees
	// reads gitdir + HEAD from the entry dir.
	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(tmp, "feat-x-checkout")
	if err := os.MkdirAll(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(siteDir, ".git", "worktrees", "feat-x")
	os.MkdirAll(wtMeta, 0755)
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat-x\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)

	if err := os.WriteFile(filepath.Join(siteDir, ".env"), []byte("APP_URL=https://alpha.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".env"), []byte("APP_URL=https://feat-x.alpha.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// The enable path restores HTTPS from the committed .lerd.yaml intent.
	if err := config.SaveProjectConfig(siteDir, &config.ProjectConfig{Secured: true}); err != nil {
		t.Fatalf("save .lerd.yaml: %v", err)
	}

	if err := config.AddSite(config.Site{
		Name:       "alpha",
		Path:       siteDir,
		Domains:    []string{"alpha.test"},
		PHPVersion: "8.4",
		Secured:    true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	// Seed an OLD cert at the old primary so we can verify it's torn down.
	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certsDir, "alpha.test.crt"), []byte("OLD-CERT"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certsDir, "alpha.test.key"), []byte("OLD-KEY"), 0644); err != nil {
		t.Fatal(err)
	}

	changed := migrateSiteTLD("test", "localhost", false)
	if !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("changed = %v, want [alpha]", changed)
	}

	// New cert must exist at the new primary.
	newCert := filepath.Join(certsDir, "alpha.localhost.crt")
	body, err := os.ReadFile(newCert)
	if err != nil {
		t.Fatalf("expected cert at %s, got %v", newCert, err)
	}
	// Cert SANs must include the renamed worktree wildcard.
	wantSAN := "*.feat-x.alpha.localhost"
	if !contains(body, wantSAN) {
		t.Errorf("cert %s missing SAN %q; got %q", newCert, wantSAN, body)
	}
	// Old cert files must be cleaned up so the certs/sites dir doesn't
	// accumulate stale entries across migrations.
	if _, err := os.Stat(filepath.Join(certsDir, "alpha.test.crt")); !os.IsNotExist(err) {
		t.Errorf("old cert should be removed after migration; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(certsDir, "alpha.test.key")); !os.IsNotExist(err) {
		t.Errorf("old key should be removed after migration; stat err = %v", err)
	}
}

// TestMigrateSiteTLD_DNSRoundTripRestoresHTTPS pins the fix for #749: disabling
// then re-enabling DNS must be lossless. A site secured on .test falls back to
// plain .localhost http on disable (registry flag off) but keeps its committed
// HTTPS intent in .lerd.yaml; re-enabling reads that intent and re-secures the
// site on .test, reissuing the cert and syncing the .env back to https.
func TestMigrateSiteTLD_DNSRoundTripRestoresHTTPS(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeMkcert := `#!/bin/sh
CRT=""
KEY=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -cert-file) shift; CRT="$1" ;;
    -key-file) shift; KEY="$1" ;;
  esac
  shift
done
echo "CERT" > "$CRT"
echo "KEY" > "$KEY"
`
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}

	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(siteDir, ".env")
	if err := os.WriteFile(envPath, []byte("APP_URL=https://alpha.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProjectConfig(siteDir, &config.ProjectConfig{Secured: true}); err != nil {
		t.Fatalf("save .lerd.yaml: %v", err)
	}
	if err := config.AddSite(config.Site{
		Name:       "alpha",
		Path:       siteDir,
		Domains:    []string{"alpha.test"},
		PHPVersion: "8.4",
		Secured:    true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	// Disable DNS: site falls back to plain-http .localhost.
	if changed := migrateSiteTLD("test", "localhost", true); !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("disable changed = %v, want [alpha]", changed)
	}
	site, err := config.FindSite("alpha")
	if err != nil {
		t.Fatalf("FindSite after disable: %v", err)
	}
	if site.PrimaryDomain() != "alpha.localhost" || site.Secured {
		t.Fatalf("after disable: domain=%q secured=%v, want alpha.localhost/false", site.PrimaryDomain(), site.Secured)
	}
	if envBytes, _ := os.ReadFile(envPath); !contains(envBytes, "APP_URL=http://alpha.localhost") {
		t.Errorf("after disable .env = %q, want http://alpha.localhost", envBytes)
	}
	if proj, _ := config.LoadProjectConfig(siteDir); !proj.Secured {
		t.Fatalf("after disable: .lerd.yaml secured intent lost")
	}

	// Re-enable DNS: site returns to .test and HTTPS is restored from intent.
	if changed := migrateSiteTLD("localhost", "test", false); !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("enable changed = %v, want [alpha]", changed)
	}
	site, err = config.FindSite("alpha")
	if err != nil {
		t.Fatalf("FindSite after enable: %v", err)
	}
	if site.PrimaryDomain() != "alpha.test" || !site.Secured {
		t.Fatalf("after enable: domain=%q secured=%v, want alpha.test/true", site.PrimaryDomain(), site.Secured)
	}
	if envBytes, _ := os.ReadFile(envPath); !contains(envBytes, "APP_URL=https://alpha.test") {
		t.Errorf("after enable .env = %q, want https://alpha.test", envBytes)
	}
	certPath := filepath.Join(config.CertsDir(), "sites", "alpha.test.crt")
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("expected reissued cert at %s: %v", certPath, err)
	}
}

// TestMigrateSiteTLD_EnableLeavesPlainSiteHTTP confirms a site the user
// intentionally kept on plain http (no .lerd.yaml secured intent) is not
// promoted to HTTPS when DNS is enabled.
func TestMigrateSiteTLD_EnableLeavesPlainSiteHTTP(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}
	siteDir := filepath.Join(tmp, "beta")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(siteDir, ".env")
	if err := os.WriteFile(envPath, []byte("APP_URL=http://beta.localhost\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProjectConfig(siteDir, &config.ProjectConfig{Secured: false}); err != nil {
		t.Fatalf("save .lerd.yaml: %v", err)
	}
	if err := config.AddSite(config.Site{
		Name: "beta", Path: siteDir, Domains: []string{"beta.localhost"}, PHPVersion: "8.4",
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if changed := migrateSiteTLD("localhost", "test", false); !slices.Equal(changed, []string{"beta"}) {
		t.Fatalf("changed = %v, want [beta]", changed)
	}
	site, err := config.FindSite("beta")
	if err != nil {
		t.Fatalf("FindSite: %v", err)
	}
	if site.Secured {
		t.Errorf("plain-http site should stay unsecured after DNS enable")
	}
	if envBytes, _ := os.ReadFile(envPath); !contains(envBytes, "APP_URL=http://beta.test") {
		t.Errorf(".env = %q, want http://beta.test", envBytes)
	}
}

// A site secured only in the registry, with no .lerd.yaml to carry the intent,
// must still return to HTTPS after a disable/enable round trip. The enable path
// used to read intent solely from .lerd.yaml, so such a site stayed plain http.
func TestMigrateSiteTLD_DNSRoundTripRestoresHTTPS_NoProjectFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeMkcert := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do case \"$1\" in -cert-file) shift; echo CERT > \"$1\" ;; -key-file) shift; echo KEY > \"$1\" ;; esac; shift; done\n"
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}

	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, ".env"), []byte("APP_URL=https://alpha.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Deliberately NO .lerd.yaml: the site's HTTPS state lives only in the registry.
	if err := config.AddSite(config.Site{
		Name: "alpha", Path: siteDir, Domains: []string{"alpha.test"}, PHPVersion: "8.4", Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if changed := migrateSiteTLD("test", "localhost", true); !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("disable changed = %v, want [alpha]", changed)
	}
	if site, _ := config.FindSite("alpha"); site.Secured {
		t.Fatalf("after disable the site should be plain http")
	}

	if changed := migrateSiteTLD("localhost", "test", false); !slices.Equal(changed, []string{"alpha"}) {
		t.Fatalf("enable changed = %v, want [alpha]", changed)
	}
	site, err := config.FindSite("alpha")
	if err != nil {
		t.Fatalf("FindSite after enable: %v", err)
	}
	if !site.Secured {
		t.Errorf("a registry-secured site with no .lerd.yaml must return to HTTPS after a DNS round trip")
	}
}

// Disabling DNS while sites use a custom TLD (which toggledCanonicalTLD leaves
// unchanged) must still drop them off HTTPS: the custom-TLD path used to return
// early, leaving sites on HTTPS-only vhosts nginx keeps serving after DNS is gone.
func TestApplyDNSTLDMigration_CustomTLDDisableUnsecures(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}
	certsDir := filepath.Join(config.CertsDir(), "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".crt", ".key"} {
		if err := os.WriteFile(filepath.Join(certsDir, "alpha.dev"+ext), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "alpha", Path: siteDir, Domains: []string{"alpha.dev"}, PHPVersion: "8.4", Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if got := applyDNSTLDMigration("dev", false); got != "dev" {
		t.Fatalf("custom TLD must be preserved on disable, got %q", got)
	}
	site, err := config.FindSite("alpha")
	if err != nil {
		t.Fatalf("FindSite: %v", err)
	}
	if site.Secured {
		t.Errorf("a custom-TLD site must be unsecured when DNS is disabled")
	}
	if _, err := os.Stat(filepath.Join(certsDir, "alpha.dev.crt")); !os.IsNotExist(err) {
		t.Errorf("cert should be removed when the site is unsecured on DNS disable")
	}
}

// On a custom-TLD disable, a site's worktree vhosts must be regenerated to plain
// http, not left on an ssl vhost whose wildcard cert was just removed. Without
// the regeneration the worktree is unserved until the watcher reconciles.
func TestApplyDNSTLDMigration_CustomTLDDisableRegeneratesWorktreeVhosts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := os.MkdirAll(config.NginxConfD(), 0755); err != nil {
		t.Fatal(err)
	}
	// A git worktree the DetectWorktrees scan will find (gitdir + HEAD).
	siteDir := filepath.Join(tmp, "app")
	checkout := filepath.Join(tmp, "feature-checkout")
	if err := os.MkdirAll(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(siteDir, ".git", "worktrees", "feature")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feature\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)
	os.WriteFile(filepath.Join(checkout, ".env"), []byte("APP_URL=https://feature.app.dev\n"), 0644)

	// A stale SSL worktree vhost that must be replaced by a plain-http one.
	staleSSL := filepath.Join(config.NginxConfD(), "feature.app.dev-ssl.conf")
	if err := os.WriteFile(staleSSL, []byte("server {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := config.AddSite(config.Site{
		Name: "app", Path: siteDir, Domains: []string{"app.dev"}, PHPVersion: "8.4", Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	applyDNSTLDMigration("dev", false)

	if _, err := os.Stat(staleSSL); !os.IsNotExist(err) {
		t.Errorf("stale ssl worktree vhost must be removed on disable; stat err = %v", err)
	}
	httpConf := filepath.Join(config.NginxConfD(), "feature.app.dev.conf")
	if _, err := os.Stat(httpConf); err != nil {
		t.Errorf("worktree must be regenerated as a plain-http vhost: %v", err)
	}
}

func TestMigrateSiteTLD_NoOpWhenSameTLD(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := config.AddSite(config.Site{
		Name: "x", Path: filepath.Join(tmp, "x"), Domains: []string{"x.test"},
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if got := migrateSiteTLD("test", "test", false); got != nil {
		t.Errorf("noop expected, got %v", got)
	}
}

func contains(haystack []byte, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}
