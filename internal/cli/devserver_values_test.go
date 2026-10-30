package cli

import (
	"os"
	"os/exec"
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
		Origin:         "https://winter.test",
		Hosts:          []string{"winter.test", ".winter.test"},
		OriginPatterns: []string{`^https://([a-z0-9-]+\.)*winter\.test$`},
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
		Origin:         "https://winter.test",
		Hosts:          []string{"winter.test"},
		OriginPatterns: []string{`^https://([a-z0-9-]+\.)*winter\.test$`},
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

// The patterns only work once the shipped config turns them into RegExps, so
// load the real module and ask its origin list what a tenant page gets.
func TestShippedDevServerValuesLetSubdomainsFetch(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	addr := devServerAddrFor(true, "myapp.test", []string{"myapp.test"})
	rel, err := writeDevServerValues(dir, viteToolForTest(t), addr, 5173)
	if err != nil {
		t.Fatal(err)
	}
	script := `import(process.argv[1]).then((m) => {
	const list = m.default.server.cors.origin;
	const ok = (u) => list.some((p) => (p instanceof RegExp ? p.test(u) : p === u));
	console.log(["https://myapp.test", "https://tenant.myapp.test", "https://evilmyapp.test", "http://tenant.myapp.test"].map(ok).join(" "));
});`
	out, err := exec.Command("node", "-e", script, filepath.Join(dir, rel)).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "true true false false" {
		t.Errorf("apex, tenant, look-alike, wrong scheme allowed = %q, want true true false false", got)
	}
}

func TestShippedDevServerWrapperTurnsPatternsIntoRegExps(t *testing.T) {
	dir := gitRepo(t, "/node_modules\n")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := writeDevServerWrapper(dir, viteToolForTest(t), devServerAddrFor(true, "myapp.test", []string{"myapp.test"}))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, rel))
	if !strings.Contains(string(body), ".map((p) => new RegExp(p))") {
		t.Errorf("shipped wrapper leaves the origin patterns as strings:\n%s", body)
	}
}

// A .env written while a slow plugin is still starting makes Vite restart
// before its first listen, and the startup then binds the port a second time.
// Load the real wrapper against a stub vite and ask when the restart runs.
func TestShippedDevServerWrapperHoldsRestartUntilListening(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	dir := gitRepo(t, "/node_modules\n")
	stub := map[string]string{
		"vite.config.js":                 "export default {}\n",
		"node_modules/vite/package.json": `{"name":"vite","type":"module","main":"index.js"}`,
		"node_modules/vite/index.js": "export const mergeConfig = (a, b) => ({ ...a, ...b });\n" +
			"export const loadConfigFromFile = async () => null;\n",
	}
	for name, body := range stub {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rel, err := writeDevServerWrapper(dir, viteToolForTest(t), devServerAddrFor(true, "myapp.test", []string{"myapp.test"}))
	if err != nil {
		t.Fatal(err)
	}
	script := `import { EventEmitter } from 'node:events';
import(process.argv[1]).then(async (m) => {
	const config = await m.default({ command: 'serve' });
	const httpServer = Object.assign(new EventEmitter(), { listening: false, address: () => ({ port: 5173 }) });
	const log = [];
	const server = { httpServer, restart: async () => log.push('restart') };
	for (const p of config.plugins) p.configureServer?.(server);
	const pending = server.restart();
	await new Promise((r) => setTimeout(r, 20));
	log.push('listening');
	httpServer.listening = true;
	httpServer.emit('listening');
	await pending;
	await server.restart();
	console.log(log.join(' '));
});`
	out, err := exec.Command("node", "-e", script, filepath.Join(dir, rel)).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "listening restart restart" {
		t.Errorf("order = %q, want the first restart held until listening, then later ones straight through", got)
	}
}
