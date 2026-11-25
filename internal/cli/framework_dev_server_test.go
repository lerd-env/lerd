package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/linker"
)

// nodeFrameworkProject sandboxes lerd's config with a dev-server framework and
// returns a project that only its package.json identifies.
func nodeFrameworkProject(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	def := "name: acmejs\nlabel: AcmeJS\ndetect:\n  - package: {file: package.json, name: \"@acme/core\"}\ndev_server:\n  command: npm run acme\n  port: 43117\n"
	if err := os.MkdirAll(config.FrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.FrameworksDir(), "acmejs.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := `{"scripts":{"start:dev":"x"},"dependencies":{"@acme/core":"^1"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDevServerFramework_isOfferedAsAProxy(t *testing.T) {
	dir := nodeFrameworkProject(t)
	kind, choice, _, _ := projectKindFor(dir, &config.ProjectConfig{})
	if kind != ProjectKindProxy || choice {
		t.Errorf("kind = %q choice = %v, want proxy with no question", kind, choice)
	}
	var q ProjectQuestions
	fillProxyQuestions(&q, dir, &config.ProjectConfig{})
	if q.ProxyCommand != "npm run acme" || q.ProxyPort != 43117 {
		t.Errorf("command/port = %q/%d, want the definition's", q.ProxyCommand, q.ProxyPort)
	}
	saved := &config.ProjectConfig{Proxy: &config.ProxyConfig{Command: "mine", Port: 4000}}
	fillProxyQuestions(&q, dir, saved)
	if q.ProxyCommand != "mine" || q.ProxyPort != 4000 {
		t.Errorf("command/port = %q/%d, want the saved ones", q.ProxyCommand, q.ProxyPort)
	}
}

func TestDevServerFramework_isNeverParkedOrLinkedAsPHP(t *testing.T) {
	dir := nodeFrameworkProject(t)
	if parkAdmits(dir) {
		t.Error("an unattended park admitted a project served by a host command")
	}
	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	_, err := linker.Resolve(dir, cfg, linker.CLIPolicy("", false, nil))
	if err == nil || !strings.Contains(err.Error(), "lerd init") {
		t.Errorf("err = %v, want a refusal naming lerd init", err)
	}
}

func TestNoContainerForWorker(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, s := range []config.Site{
		{Name: "api", Domains: []string{"api.test"}, Path: t.TempDir(), HostPort: 3000},
		{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), PHPVersion: "8.4"},
	} {
		if err := config.AddSite(s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := prepareWorkerStart("api", t.TempDir(), "", "queue", config.FrameworkWorker{Command: "true"}); err == nil || !strings.Contains(err.Error(), "host: true") {
		t.Errorf("err = %v, want a refusal naming host: true", err)
	}
	if msg := noContainerForWorkerMsg("api", "queue", config.FrameworkWorker{Host: true}); msg != "" {
		t.Errorf("a host worker was refused: %s", msg)
	}
	if msg := noContainerForWorkerMsg("shop", "queue", config.FrameworkWorker{}); msg != "" {
		t.Errorf("a PHP site's worker was refused: %s", msg)
	}
}

// The scaffold command sits where an older lerd does not look for it.
func TestDevServerFramework_scaffolds(t *testing.T) {
	nodeFrameworkProject(t)
	path := filepath.Join(config.FrameworksDir(), "acmejs.yaml")
	def, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(def, "  create: npx @acme/cli new\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	fw, ok := config.GetFrameworkForScaffold("acmejs", "")
	if !ok || fw.ScaffoldCommand() != "npx @acme/cli new" || fw.Create != "" {
		t.Errorf("create = %q / %q (%v)", fw.Create, fw.ScaffoldCommand(), ok)
	}
}
