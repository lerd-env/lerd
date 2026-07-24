package cli

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── isTextContent ─────────────────────────────────────────────────────────────

func TestIsTextContent(t *testing.T) {
	cases := []struct {
		ct   string
		want bool
	}{
		{"text/html; charset=utf-8", true},
		{"text/html", true},
		{"text/css", true},
		{"application/javascript", true},
		{"text/javascript", true},
		{"application/json", true},
		{"image/png", false},
		{"image/jpeg", false},
		{"application/octet-stream", false},
		{"application/pdf", false},
		{"", false},
	}
	for _, c := range cases {
		got := isTextContent(c.ct)
		if got != c.want {
			t.Errorf("isTextContent(%q) = %v, want %v", c.ct, got, c.want)
		}
	}
}

// ── pickShareTool ─────────────────────────────────────────────────────────────

// fakeBin creates a fake executable named binName in a temp dir and returns
// a PATH that contains only that directory (plus an optional extra dir).
func fakeBin(t *testing.T, binNames ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range binNames {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPickShareTool_mutualExclusion(t *testing.T) {
	pairs := [][5]bool{
		{true, true, false, false, false}, // ngrok + cloudflare
		{true, false, true, false, false}, // ngrok + expose
		{false, true, true, false, false}, // cloudflare + expose
		{false, false, true, true, false}, // expose + serveo
		{true, false, false, false, true}, // ngrok + localhost-run
	}
	for _, p := range pairs {
		_, err := pickShareTool(p[0], p[1], p[2], p[3], p[4], "", "")
		if err == nil {
			t.Errorf("pickShareTool%v: expected mutual-exclusion error, got nil", p)
			continue
		}
		if !strings.Contains(err.Error(), "only one of") {
			t.Errorf("pickShareTool%v: error %q does not contain 'only one of'", p, err)
		}
	}
}

func TestPickShareTool_explicitNgrok_present(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok"))
	tool, err := pickShareTool(true, false, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeNgrok {
		t.Errorf("mode = %v, want shareModeNgrok", tool.mode)
	}
}

func TestPickShareTool_explicitNgrok_absent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(true, false, false, false, false, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ngrok not found") {
		t.Errorf("error %q does not mention ngrok", err)
	}
}

func TestPickShareTool_explicitCloudflare_present(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "cloudflared"))
	tool, err := pickShareTool(false, true, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeCloudflare {
		t.Errorf("mode = %v, want shareModeCloudflare", tool.mode)
	}
}

func TestPickShareTool_explicitCloudflare_absent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(false, true, false, false, false, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cloudflared not found") {
		t.Errorf("error %q does not mention cloudflared", err)
	}
}

func TestPickShareTool_explicitExpose_present(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "expose"))
	tool, err := pickShareTool(false, false, true, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeExpose {
		t.Errorf("mode = %v, want shareModeExpose", tool.mode)
	}
}

func TestPickShareTool_explicitExpose_absent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(false, false, true, false, false, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "expose not found") {
		t.Errorf("error %q does not mention expose", err)
	}
}

func TestPickShareTool_explicitServeo(t *testing.T) {
	tool, err := pickShareTool(false, false, false, true, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeSSH {
		t.Errorf("mode = %v, want shareModeSSH", tool.mode)
	}
	if tool.sshHost != "serveo.net" {
		t.Errorf("sshHost = %q, want serveo.net", tool.sshHost)
	}
}

func TestPickShareTool_explicitLocalhostRun(t *testing.T) {
	tool, err := pickShareTool(false, false, false, false, true, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeSSH {
		t.Errorf("mode = %v, want shareModeSSH", tool.mode)
	}
	if tool.sshHost != "localhost.run" {
		t.Errorf("sshHost = %q, want localhost.run", tool.sshHost)
	}
}

func TestPickShareTool_autoDetect_ngrokFirst(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared", "expose", "ssh"))
	tool, err := pickShareTool(false, false, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeNgrok {
		t.Errorf("mode = %v, want shareModeNgrok (ngrok takes priority)", tool.mode)
	}
}

func TestPickShareTool_autoDetect_cloudflareBeforeExpose(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "cloudflared", "expose", "ssh"))
	tool, err := pickShareTool(false, false, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeCloudflare {
		t.Errorf("mode = %v, want shareModeCloudflare", tool.mode)
	}
}

func TestPickShareTool_autoDetect_expose(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "expose", "ssh"))
	tool, err := pickShareTool(false, false, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeExpose {
		t.Errorf("mode = %v, want shareModeExpose", tool.mode)
	}
}

func TestPickShareTool_autoDetect_sshFallback(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ssh"))
	tool, err := pickShareTool(false, false, false, false, false, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeSSH {
		t.Errorf("mode = %v, want shareModeSSH", tool.mode)
	}
	if tool.sshHost != "localhost.run" {
		t.Errorf("sshHost = %q, want localhost.run", tool.sshHost)
	}
}

func TestPickShareTool_autoDetect_noTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(false, false, false, false, false, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no tunnel tool found") {
		t.Errorf("error %q does not mention 'no tunnel tool found'", err)
	}
}

func TestPickShareTool_domain_withExplicitCloudflare(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	tool, err := pickShareTool(false, true, false, false, false, "dev.example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeCloudflare {
		t.Errorf("mode = %v, want shareModeCloudflare", tool.mode)
	}
	if tool.domain != "dev.example.com" {
		t.Errorf("domain = %q, want dev.example.com", tool.domain)
	}
}

func TestPickShareTool_domain_withCloudflareDefault(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	tool, err := pickShareTool(false, false, false, false, false, "dev.example.com", "cloudflare")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeCloudflare {
		t.Errorf("mode = %v, want shareModeCloudflare", tool.mode)
	}
	if tool.domain != "dev.example.com" {
		t.Errorf("domain = %q, want dev.example.com", tool.domain)
	}
}

func TestPickShareTool_domain_impliesCloudflare(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	// --domain is Cloudflare-only, so it selects the tool with no flag at all and
	// outranks a configured default pointing somewhere else.
	for _, defaultTool := range []string{"", "ngrok"} {
		tool, err := pickShareTool(false, false, false, false, false, "dev.example.com", defaultTool)
		if err != nil {
			t.Errorf("default %q: unexpected error: %v", defaultTool, err)
			continue
		}
		if tool.mode != shareModeCloudflare {
			t.Errorf("default %q: mode = %v, want shareModeCloudflare", defaultTool, tool.mode)
		}
		if tool.domain != "dev.example.com" {
			t.Errorf("default %q: domain = %q, want dev.example.com", defaultTool, tool.domain)
		}
	}
}

func TestPickShareTool_domain_withOtherTool(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	for _, p := range [][4]bool{
		{true, false, false, false},
		{false, true, false, false},
		{false, false, true, false},
		{false, false, false, true},
	} {
		_, err := pickShareTool(p[0], false, p[1], p[2], p[3], "dev.example.com", "")
		if err == nil || !strings.Contains(err.Error(), "--domain only works with Cloudflare Tunnel") {
			t.Errorf("pickShareTool%v: error = %v, want '--domain only works with Cloudflare Tunnel'", p, err)
		}
	}
}

func TestPickShareTool_domain_cloudflaredAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(false, true, false, false, false, "dev.example.com", "")
	if err == nil || !strings.Contains(err.Error(), "cloudflared not found") {
		t.Errorf("error = %v, want 'cloudflared not found'", err)
	}
}

func TestPickShareTool_defaultTool_used(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared", "expose", "ssh"))
	cases := []struct {
		defaultTool string
		wantMode    shareMode
		wantSSHHost string
	}{
		{"ngrok", shareModeNgrok, ""},
		{"cloudflare", shareModeCloudflare, ""},
		{"expose", shareModeExpose, ""},
		{"serveo", shareModeSSH, "serveo.net"},
		{"localhost-run", shareModeSSH, "localhost.run"},
	}
	for _, c := range cases {
		tool, err := pickShareTool(false, false, false, false, false, "", c.defaultTool)
		if err != nil {
			t.Errorf("default %q: unexpected error: %v", c.defaultTool, err)
			continue
		}
		if tool.mode != c.wantMode {
			t.Errorf("default %q: mode = %v, want %v", c.defaultTool, tool.mode, c.wantMode)
		}
		if tool.sshHost != c.wantSSHHost {
			t.Errorf("default %q: sshHost = %q, want %q", c.defaultTool, tool.sshHost, c.wantSSHHost)
		}
	}
}

func TestPickShareTool_defaultTool_flagOverrides(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	tool, err := pickShareTool(true, false, false, false, false, "", "cloudflare")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeNgrok {
		t.Errorf("mode = %v, want shareModeNgrok (flag beats default)", tool.mode)
	}
}

func TestPickShareTool_defaultTool_missingBinary_namesTheDefault(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, c := range []struct{ defaultTool, wantBinary string }{
		{"ngrok", "ngrok"}, {"cloudflare", "cloudflared"}, {"expose", "expose"},
	} {
		_, err := pickShareTool(false, false, false, false, false, "", c.defaultTool)
		if err == nil {
			t.Errorf("default %q: expected an error, got nil", c.defaultTool)
			continue
		}
		if !strings.Contains(err.Error(), c.wantBinary+" not found") {
			t.Errorf("default %q: error = %v, want %q not found", c.defaultTool, err, c.wantBinary)
		}
		if !strings.Contains(err.Error(), "lerd share:tool") {
			t.Errorf("default %q: error should point at share:tool, got %v", c.defaultTool, err)
		}
	}
}

func TestPickShareTool_explicitFlag_missingBinary_omitsTheHint(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickShareTool(true, false, false, false, false, "", "")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if strings.Contains(err.Error(), "share:tool") {
		t.Errorf("an explicit --ngrok should not mention the default, got %v", err)
	}
}

func TestPickShareTool_defaultTool_unknown(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok"))
	_, err := pickShareTool(false, false, false, false, false, "", "bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown default share tool") {
		t.Errorf("error = %v, want 'unknown default share tool'", err)
	}
}

// ── ensureCloudflareTunnel ────────────────────────────────────────────────────

// fakeTunnelID is the id the fake cloudflared reports from "tunnel list", and so
// the stem of the credentials file ensureCloudflareTunnel looks for.
const fakeTunnelID = "11111111-2222-3333-4444-555555555555"

// fakeCloudflared installs a fake cloudflared script and returns its log file.
// The script logs every invocation; createExit/routeExit control the exit codes
// and output of "tunnel create" / "tunnel route dns". "tunnel login" writes
// cert.pem under $HOME/.cloudflared.
func fakeCloudflared(t *testing.T, createOut string, createExit int, routeOut string, routeExit int) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.log")
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %q
case "$1 $2" in
"tunnel login") : > "$HOME/.cloudflared/cert.pem";;
"tunnel create") echo %q; exit %d;;
"tunnel list") printf '[{"id":"%s","name":"%%s"}]\n' "$4";;
"tunnel route") echo %q; exit %d;;
esac
`, logFile, createOut, createExit, fakeTunnelID, routeOut, routeExit)
	if err := os.WriteFile(filepath.Join(dir, "cloudflared"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return logFile
}

// certWithTunnelCredentials points TUNNEL_ORIGIN_CERT at a fresh cert with the
// credentials file for fakeTunnelID beside it, the state of a machine that
// created the tunnel itself.
func certWithTunnelCredentials(t *testing.T) {
	t.Helper()
	dir := newCertDir(t)
	if err := os.WriteFile(filepath.Join(dir, fakeTunnelID+".json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
}

// certWithoutTunnelCredentials leaves the credentials file out, the state of a
// machine where the tunnel was created somewhere else.
func certWithoutTunnelCredentials(t *testing.T) {
	t.Helper()
	newCertDir(t)
}

func newCertDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNNEL_ORIGIN_CERT", filepath.Join(dir, "cert.pem"))
	return dir
}

func readCalls(t *testing.T, logFile string) string {
	t.Helper()
	b, _ := os.ReadFile(logFile)
	return string(b)
}

func TestEnsureCloudflareTunnel_happyPath_logsInWhenNoCert(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TUNNEL_ORIGIN_CERT", "")
	if err := os.MkdirAll(filepath.Join(home, ".cloudflared"), 0755); err != nil {
		t.Fatal(err)
	}
	logFile := fakeCloudflared(t, "created", 0, "routed", 0)

	name, err := ensureCloudflareTunnel("mysite", "dev.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "lerd-mysite" {
		t.Errorf("name = %q, want lerd-mysite", name)
	}
	calls := readCalls(t, logFile)
	for _, want := range []string{"tunnel login", "tunnel create lerd-mysite", "tunnel route dns lerd-mysite dev.example.com"} {
		if !strings.Contains(calls, want) {
			t.Errorf("calls %q missing %q", calls, want)
		}
	}
}

func TestEnsureCloudflareTunnel_skipsLoginWhenCertExists(t *testing.T) {
	cert := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(cert, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNNEL_ORIGIN_CERT", cert)
	logFile := fakeCloudflared(t, "created", 0, "routed", 0)

	if _, err := ensureCloudflareTunnel("mysite", "dev.example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(readCalls(t, logFile), "tunnel login") {
		t.Error("tunnel login was called despite existing cert")
	}
}

func TestEnsureCloudflareTunnel_toleratesExistingTunnelAndRoute(t *testing.T) {
	certWithTunnelCredentials(t)
	fakeCloudflared(t, "tunnel with name already exists", 1, "record with that host already exists", 1)

	var name string
	var err error
	out := captureStdout(t, func() { name, err = ensureCloudflareTunnel("mysite", "dev.example.com") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "lerd-mysite" {
		t.Errorf("name = %q, want lerd-mysite", name)
	}
	if !strings.Contains(out, "already exists") {
		t.Errorf("expected a note about the existing record, got %q", out)
	}
}

func TestEnsureCloudflareTunnel_existingTunnelMissingCredentials(t *testing.T) {
	certWithoutTunnelCredentials(t)
	fakeCloudflared(t, "tunnel with name already exists", 1, "routed", 0)

	_, err := ensureCloudflareTunnel("mysite", "dev.example.com")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "credentials file is missing") {
		t.Errorf("error = %v, want it to name the missing credentials file", err)
	}
	if !strings.Contains(err.Error(), fakeTunnelID+".json") {
		t.Errorf("error = %v, want it to name the expected credentials path", err)
	}
}

func TestEnsureCloudflareTunnel_freshRouteWarnsAboutPropagation(t *testing.T) {
	certWithTunnelCredentials(t)
	fakeCloudflared(t, "created", 0, "Added CNAME dev.example.com which will route to this tunnel", 0)

	out := captureStdout(t, func() {
		if _, err := ensureCloudflareTunnel("mysite", "dev.example.com"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "30 minutes") {
		t.Errorf("a freshly added record should warn about resolver caching, got %q", out)
	}
}

func TestEnsureCloudflareTunnel_reusedRouteStaysQuiet(t *testing.T) {
	certWithTunnelCredentials(t)
	fakeCloudflared(t, "created", 0, "", 0)

	out := captureStdout(t, func() {
		if _, err := ensureCloudflareTunnel("mysite", "dev.example.com"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if strings.Contains(out, "30 minutes") {
		t.Errorf("an unchanged route should not warn about propagation, got %q", out)
	}
}

func TestEnsureCloudflareTunnel_createFailurePropagates(t *testing.T) {
	cert := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(cert, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNNEL_ORIGIN_CERT", cert)
	fakeCloudflared(t, "auth error", 1, "routed", 0)

	_, err := ensureCloudflareTunnel("mysite", "dev.example.com")
	if err == nil || !strings.Contains(err.Error(), "tunnel create") {
		t.Errorf("error = %v, want tunnel create failure", err)
	}
}

func TestEnsureCloudflareTunnel_routeFailurePropagates(t *testing.T) {
	cert := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(cert, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUNNEL_ORIGIN_CERT", cert)
	fakeCloudflared(t, "created", 0, "zone not found", 1)

	_, err := ensureCloudflareTunnel("mysite", "dev.example.com")
	if err == nil || !strings.Contains(err.Error(), "route dns") {
		t.Errorf("error = %v, want route dns failure", err)
	}
}

// ── startHostProxy ────────────────────────────────────────────────────────────

// proxyPort parses the port from a URL string like "http://127.0.0.1:PORT".
func proxyPort(t *testing.T, rawURL string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(rawURL, "http://"), "https://"))
	if err != nil {
		t.Fatalf("parsing port from %q: %v", rawURL, err)
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// doProxy sends a GET to the proxy with the given Host header and returns the response.
func doProxy(t *testing.T, port int, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	return resp
}

func TestStartHostProxy_rewritesHostHeader(t *testing.T) {
	var gotHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	resp.Body.Close()

	if gotHost != "mysite.test" {
		t.Errorf("backend Host = %q, want %q", gotHost, "mysite.test")
	}
}

func TestStartHostProxy_setsForwardedHeaders(t *testing.T) {
	var gotXFH, gotXFP string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFH = r.Header.Get("X-Forwarded-Host")
		gotXFP = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	resp.Body.Close()

	if gotXFH != "abc.trycloudflare.com" {
		t.Errorf("X-Forwarded-Host = %q, want %q", gotXFH, "abc.trycloudflare.com")
	}
	if gotXFP != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want https", gotXFP)
	}
}

func TestStartHostProxy_rewritesLocationHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://mysite.test/dashboard")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	req.Host = "abc.trycloudflare.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "abc.trycloudflare.com") {
		t.Errorf("Location = %q, want it to contain tunnel host", loc)
	}
	if strings.Contains(loc, "mysite.test") {
		t.Errorf("Location = %q still contains local domain", loc)
	}
}

func TestStartHostProxy_rewritesLocationHeader_httpToHttps(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://mysite.test/login")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	req.Host = "tunnel.example.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://") {
		t.Errorf("Location = %q, want https:// prefix after rewrite", loc)
	}
}

func TestStartHostProxy_rewritesHTMLBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<a href="http://mysite.test/page">link</a>`)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if strings.Contains(string(body), "mysite.test") {
		t.Errorf("body still contains local domain: %s", body)
	}
	if !strings.Contains(string(body), "abc.trycloudflare.com") {
		t.Errorf("body does not contain tunnel host: %s", body)
	}
}

func TestStartHostProxy_doesNotRewriteBinaryBody(t *testing.T) {
	original := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(original)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != string(original) {
		t.Errorf("binary body was modified")
	}
}

func TestStartHostProxy_listensOnLoopback(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	// Must be reachable on 127.0.0.1 (not just ::1)
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("proxy not reachable on 127.0.0.1: %v", err)
	}
	resp.Body.Close()
}

func TestStartHostProxy_requestsIdentityEncoding(t *testing.T) {
	var gotAE string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAE = r.Header.Get("Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	resp.Body.Close()

	if gotAE != "identity" {
		t.Errorf("upstream Accept-Encoding = %q, want %q", gotAE, "identity")
	}
}

func TestStartHostProxy_rewritesGzippedHTMLBody(t *testing.T) {
	original := `<link rel="stylesheet" href="http://mysite.test/app.css"><script src="http://mysite.test/app.js"></script>`
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(original)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	gw.Close()
	compressed := buf.Bytes()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("Content-Encoding = %q, want empty after rewrite", resp.Header.Get("Content-Encoding"))
	}
	if strings.Contains(string(body), "mysite.test") {
		t.Errorf("body still contains local domain: %s", body)
	}
	if !strings.Contains(string(body), "abc.trycloudflare.com") {
		t.Errorf("body does not contain tunnel host: %s", body)
	}
}

func TestStartHostProxy_stopClosesListener(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	stop()

	_, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err == nil {
		t.Error("expected connection error after stop, got nil")
	}
}
