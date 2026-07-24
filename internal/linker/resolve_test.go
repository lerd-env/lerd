package linker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// testConfig is the global config the resolve tests link against: lerd manages
// DNS on the .test TLD, so secured decisions are reachable.
func testConfig() *config.GlobalConfig {
	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	cfg.DNS.Enabled = true
	cfg.PHP.DefaultVersion = "8.3"
	cfg.Node.DefaultVersion = "22"
	return cfg
}

// projectDir creates a project directory named name, with the given .lerd.yaml
// body when one is wanted, and sandboxes the site registry and global config
// around it. The global config is written to disk because PHP detection falls
// back to reading it directly rather than to the config Resolve is handed.
func projectDir(t *testing.T, name, lerdYAML string) string {
	t.Helper()
	setupSitesYAML(t, "sites: []\n")
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	if err := os.MkdirAll(filepath.Join(cfgHome, "lerd"), 0755); err != nil {
		t.Fatal(err)
	}
	global := "php:\n  default_version: \"8.3\"\nnode:\n  default_version: \"22\"\ndns:\n  enabled: true\n  tld: test\n"
	if err := os.WriteFile(filepath.Join(cfgHome, "lerd", "config.yaml"), []byte(global), 0644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if lerdYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(lerdYAML), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolve_plainProjectServesOverFPM(t *testing.T) {
	dir := projectDir(t, "myapp", "")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Registered() {
		t.Fatalf("skipped unexpectedly: %s", plan.SkipDetail)
	}
	if plan.Mode != ModeFPM {
		t.Errorf("mode = %q, want %q", plan.Mode, ModeFPM)
	}
	if plan.Site.Name != "myapp" {
		t.Errorf("name = %q, want myapp", plan.Site.Name)
	}
	if !sliceEq(plan.Site.Domains, []string{"myapp.test"}) {
		t.Errorf("domains = %v, want [myapp.test]", plan.Site.Domains)
	}
	if plan.Site.PHPVersion != "8.3" {
		t.Errorf("php = %q, want 8.3", plan.Site.PHPVersion)
	}
}

func TestResolve_customContainerSkipsPHPDetection(t *testing.T) {
	dir := projectDir(t, "api", "container:\n  port: 3000\n  ssl: true\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != ModeCustomContainer {
		t.Fatalf("mode = %q, want %q", plan.Mode, ModeCustomContainer)
	}
	if plan.Site.ContainerPort != 3000 || !plan.Site.ContainerSSL {
		t.Errorf("container port/ssl = %d/%v, want 3000/true", plan.Site.ContainerPort, plan.Site.ContainerSSL)
	}
	if plan.Site.PHPVersion != "" {
		t.Errorf("php = %q, want empty for a proxied container", plan.Site.PHPVersion)
	}
}

func TestResolve_hostProxyCarriesTheCommandForConsent(t *testing.T) {
	dir := projectDir(t, "web", "proxy:\n  port: 5173\n  command: npm run dev\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != ModeHostProxy {
		t.Fatalf("mode = %q, want %q", plan.Mode, ModeHostProxy)
	}
	if plan.Site.HostPort != 5173 {
		t.Errorf("host port = %d, want 5173", plan.Site.HostPort)
	}
	if plan.ProxyCommand != "npm run dev" {
		t.Errorf("proxy command = %q, want %q", plan.ProxyCommand, "npm run dev")
	}
}

func TestResolve_containerWithoutPortIsCustomFPM(t *testing.T) {
	dir := projectDir(t, "legacy", "container:\n  containerfile: Containerfile.lerd\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != ModeCustomFPM {
		t.Fatalf("mode = %q, want %q", plan.Mode, ModeCustomFPM)
	}
	if !plan.Site.IsCustomFPM() {
		t.Errorf("runtime = %q, want fpm-custom", plan.Site.Runtime)
	}
}

func TestResolve_frankenPHPIsHonouredWhenAnImageExists(t *testing.T) {
	dir := projectDir(t, "fast", "runtime: frankenphp\nruntime_worker: true\nphp_version: \"8.4\"\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != ModeFrankenPHP {
		t.Fatalf("mode = %q, want %q", plan.Mode, ModeFrankenPHP)
	}
	if !plan.Site.RuntimeWorker {
		t.Error("runtime worker = false, want true")
	}
	if plan.FrankenPHPDeclined {
		t.Error("FrankenPHPDeclined = true, want false for a supported version")
	}
}

func TestResolve_frankenPHPFallsBackToFPMBelowTheImageFloor(t *testing.T) {
	dir := projectDir(t, "old", "runtime: frankenphp\nphp_version: \"8.1\"\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != ModeFPM {
		t.Fatalf("mode = %q, want %q", plan.Mode, ModeFPM)
	}
	if !plan.FrankenPHPDeclined {
		t.Error("FrankenPHPDeclined = false, want true")
	}
	if plan.Site.Runtime != "" || plan.Site.RuntimeWorker {
		t.Errorf("runtime = %q worker = %v, want cleared", plan.Site.Runtime, plan.Site.RuntimeWorker)
	}
}

func TestResolve_securedNeedsTheCertsCapability(t *testing.T) {
	yaml := "secured: true\n"

	t.Run("granted", func(t *testing.T) {
		dir := projectDir(t, "shop", yaml)
		plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Site.Secured {
			t.Error("secured = false, want true when the policy allows certs")
		}
	})

	t.Run("withheld", func(t *testing.T) {
		dir := projectDir(t, "shop", yaml)
		plan, err := Resolve(dir, testConfig(), WatcherPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if plan.Site.Secured {
			t.Error("secured = true, want false when the policy withholds certs")
		}
	})
}

func TestResolve_watcherSkipsAnAlreadyRegisteredPath(t *testing.T) {
	dir := projectDir(t, "myapp", "")
	setupSitesYAML(t, "sites:\n  - name: myapp\n    domains:\n      - myapp.test\n    path: "+dir+"\n")

	plan, err := Resolve(dir, testConfig(), WatcherPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Skip != SkipRegistered {
		t.Fatalf("skip = %q, want %q", plan.Skip, SkipRegistered)
	}
	if plan.Registered() {
		t.Error("Registered() = true for a skipped plan")
	}
}

func TestResolve_relinkKeepsTheSamePathRegistered(t *testing.T) {
	dir := projectDir(t, "myapp", "")
	setupSitesYAML(t, "sites:\n  - name: myapp\n    domains:\n      - myapp.test\n    path: "+dir+"\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Registered() {
		t.Fatalf("skipped unexpectedly: %s", plan.SkipDetail)
	}
	if plan.Site.Name != "myapp" {
		t.Errorf("name = %q, want myapp — a re-link keeps its name", plan.Site.Name)
	}
}

func TestResolve_explicitNameBecomesThePrimaryDomain(t *testing.T) {
	dir := projectDir(t, "myapp", "domains:\n  - alias\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("chosen", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !sliceEq(plan.Site.Domains, []string{"chosen.test", "alias.test"}) {
		t.Errorf("domains = %v, want [chosen.test alias.test]", plan.Site.Domains)
	}
	if plan.Site.PrimaryDomain() != "chosen.test" {
		t.Errorf("primary = %q, want chosen.test", plan.Site.PrimaryDomain())
	}
}

func TestResolve_reportsDomainsAnotherSiteOwns(t *testing.T) {
	dir := projectDir(t, "myapp", "domains:\n  - taken\n  - free\n")
	setupSitesYAML(t, "sites:\n  - name: other\n    domains:\n      - taken.test\n    path: /projects/other\n")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !sliceEq(plan.Site.Domains, []string{"free.test"}) {
		t.Errorf("domains = %v, want [free.test]", plan.Site.Domains)
	}
	if !sliceEq(plan.DroppedDomains, []string{"taken.test"}) {
		t.Errorf("dropped = %v, want [taken.test]", plan.DroppedDomains)
	}
}

func TestResolve_publicDirFromTheProjectWinsOverDetection(t *testing.T) {
	dir := projectDir(t, "myapp", "public_dir: public_html\n")

	plan, err := Resolve(dir, testConfig(), WatcherPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Site.PublicDir != "public_html" {
		t.Errorf("public dir = %q, want public_html — the watcher must honour it too", plan.Site.PublicDir)
	}
}
