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
