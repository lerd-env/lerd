package pageinject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// lerd's own web server decides injection inside its container, which has none
// of the host's tools. A plugin whose tools the host has is still injected:
// lerd-ui serves its script on the host and checks the plugin there.
func TestPlugins_ignoresToolsTheInjectingProcessLacks(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", "/usr/bin:/bin")
	dir := filepath.Join(plugins.Dir(), "vmplug")
	if err := plugins.Scaffold("vmplug", dir); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, plugins.ManifestFile)
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(data), "requires: [curl]", "requires: [lerd-test-no-such-tool]", 1)
	if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	site := config.Site{Name: "app", PagePlugins: []string{"vmplug"}}
	if _, err := plugins.Find("vmplug"); err == nil || !strings.Contains(err.Error(), "lerd-test-no-such-tool") {
		t.Fatalf("the fixture should fail only on the missing tool, got %v", err)
	}
	if got := Plugins(site); len(got) != 1 || got[0] != "vmplug" {
		t.Fatalf("Plugins = %v; want [vmplug] even though this process lacks the tool", got)
	}
	if !strings.Contains(Tags(site, ""), "/vmplug/page.js") {
		t.Error("the page script tag is missing")
	}
}
