package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// viteToolForTest is the real vite integration, which is what the values module
// is rendered from.
func viteToolForTest(t *testing.T) *config.DevServerTool {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool := config.DevServerToolInstalled(dir)
	if tool == nil {
		t.Fatal("vite integration not found")
	}
	return tool
}

// The values module carries what a project cannot keep current by hand: the
// site's origin, the hosts the server may answer for, and the port lerd picked.
func TestWriteDevServerValues_CarriesAddressesAndPort(t *testing.T) {
	dir := t.TempDir()
	tool := viteToolForTest(t)
	addr := devServerAddr{
		Origin:  "https://winter.test",
		Hosts:   []string{"winter.test", ".winter.test"},
		Origins: []string{"https://winter.test"},
	}

	rel, err := writeDevServerValues(dir, tool, addr, 5173)
	if err != nil {
		t.Fatalf("writeDevServerValues: %v", err)
	}
	if rel != tool.ValuesPath {
		t.Errorf("path = %q, want %q", rel, tool.ValuesPath)
	}
	body, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"port: 5173",
		`origin: "https://winter.test"`,
		`allowedHosts: ["winter.test",".winter.test"]`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("values module missing %q\n%s", want, body)
		}
	}
}

// The port is a number the tool binds, not a string, so a config spreading these
// straight into vite's server block works without conversion.
func TestWriteDevServerValues_PortIsANumber(t *testing.T) {
	dir := t.TempDir()
	tool := viteToolForTest(t)

	rel, err := writeDevServerValues(dir, tool, devServerAddr{Origin: "http://app.test"}, 4321)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, rel))
	if strings.Contains(string(body), `port: "4321"`) {
		t.Errorf("port was quoted\n%s", body)
	}
}

// lerd proxies the dev server over plain HTTP, so the generated config has to
// say the server speaks HTTP and not leave it to whatever the project resolves
// to. A Herd or Valet install left on the machine carries certificates named
// after the project directory, and laravel-vite-plugin finds those and turns
// the dev server to HTTPS on their strength, which answers the vhost's
// proxy_pass with a 502 (#1858). The plugin takes `userConfig.server.https ??
// its own`, so an explicit false is what settles it.
func TestDevServerConfigKeepsTheServerOnPlainHTTP(t *testing.T) {
	dir := t.TempDir()
	tool := viteToolForTest(t)
	addr := devServerAddr{
		Origin:  "https://winter.test",
		Hosts:   []string{"winter.test"},
		Origins: []string{"https://winter.test"},
	}

	rel, err := writeDevServerValues(dir, tool, addr, 5173)
	if err != nil {
		t.Fatalf("writeDevServerValues: %v", err)
	}
	values, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "https: false") {
		t.Errorf("the values module leaves https to the project:\n%s", values)
	}

	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapRel, err := writeDevServerWrapper(dir, tool, addr)
	if err != nil {
		t.Fatalf("writeDevServerWrapper: %v", err)
	}
	wrapper, err := os.ReadFile(filepath.Join(dir, wrapRel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "https: false") {
		t.Errorf("the wrapper leaves https to the project:\n%s", wrapper)
	}
}
