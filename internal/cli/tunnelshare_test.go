package cli

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ── parseTunnelURL ────────────────────────────────────────────────────────────

func TestParseTunnelURL(t *testing.T) {
	cases := []struct {
		tool string
		line string
		want string
	}{
		{"ngrok", `{"addr":"http://localhost:80","lvl":"info","msg":"started tunnel","obj":"tunnels","url":"https://84c1-1-2-3-4.ngrok-free.app"}`, "https://84c1-1-2-3-4.ngrok-free.app"},
		{"ngrok", `{"lvl":"info","msg":"client session established","obj":"csess"}`, ""},
		{"cloudflare", `2026-07-25T08:00:00Z INF |  https://plum-lakes-agree.trycloudflare.com                        |`, "https://plum-lakes-agree.trycloudflare.com"},
		{"cloudflare", `2026-07-25T08:00:00Z INF Requesting new quick Tunnel on trycloudflare.com...`, ""},
		{"expose", `Expose-URL:         https://mysite.sharedwithexpose.com`, "https://mysite.sharedwithexpose.com"},
		{"expose", `Public HTTPS:       https://mysite.sharedwithexpose.com`, "https://mysite.sharedwithexpose.com"},
		{"expose", `Visit https://expose.dev/docs to get started`, ""},
		{"serveo", `Forwarding HTTP traffic from https://f3a1c2.serveo.net`, "https://f3a1c2.serveo.net"},
		{"serveo", `Press g to start a GUI session and ctrl-c to quit.`, ""},
		{"localhost-run", `a1b2c3d4.lhr.life tunneled with tls termination, https://a1b2c3d4.lhr.life`, "https://a1b2c3d4.lhr.life"},
		{"localhost-run", `** visit https://localhost.run/docs/ for documentation **`, ""},
	}
	for _, c := range cases {
		got, ok := parseTunnelURL(c.tool, c.line)
		if c.want == "" {
			if ok {
				t.Errorf("parseTunnelURL(%s, %q) matched %q, want no match", c.tool, c.line, got)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("parseTunnelURL(%s, %q) = %q, %v; want %q", c.tool, c.line, got, ok, c.want)
		}
	}
}

// ── tool status / auto pick ───────────────────────────────────────────────────

func TestShareTools_detectsInstalledBinaries(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "ssh"))
	installed := map[string]bool{}
	for _, tool := range ShareTools().Tools {
		installed[tool.Name] = tool.Installed
	}
	want := map[string]bool{"ngrok": true, "cloudflare": false, "expose": false, "serveo": true, "localhost-run": true}
	for name, w := range want {
		if installed[name] != w {
			t.Errorf("tool %s installed = %v, want %v", name, installed[name], w)
		}
	}
}

func TestShareTools_listsEveryToolWithLabel(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	tools := ShareTools().Tools
	if len(tools) != len(shareTools) {
		t.Fatalf("got %d tools, want %d", len(tools), len(shareTools))
	}
	for _, tool := range tools {
		if tool.Label == "" || tool.Binary == "" {
			t.Errorf("tool %s missing label or binary: %+v", tool.Name, tool)
		}
	}
}

func TestAutoShareToolName(t *testing.T) {
	cases := []struct {
		bins        []string
		defaultTool string
		want        string
	}{
		{[]string{"ngrok", "cloudflared"}, "", "ngrok"},
		{[]string{"cloudflared", "expose"}, "", "cloudflare"},
		{[]string{"expose", "ssh"}, "", "expose"},
		{[]string{"ssh"}, "", "localhost-run"},
		{[]string{"ngrok", "cloudflared"}, "cloudflare", "cloudflare"},
		{[]string{"ngrok"}, "cloudflare", "ngrok"}, // default not installed, fall back to auto
		{nil, "", ""},
	}
	for _, c := range cases {
		t.Setenv("PATH", fakeBin(t, c.bins...))
		if got := autoShareToolName(c.defaultTool); got != c.want {
			t.Errorf("autoShareToolName(%q) with %v = %q, want %q", c.defaultTool, c.bins, got, c.want)
		}
	}
}

func TestShareToolCanonicalName(t *testing.T) {
	cases := []struct {
		tool *shareTool
		want string
	}{
		{&shareTool{mode: shareModeNgrok}, "ngrok"},
		{&shareTool{mode: shareModeCloudflare}, "cloudflare"},
		{&shareTool{mode: shareModeExpose}, "expose"},
		{&shareTool{mode: shareModeSSH, sshHost: "serveo.net"}, "serveo"},
		{&shareTool{mode: shareModeSSH, sshHost: "localhost.run"}, "localhost-run"},
	}
	for _, c := range cases {
		if got := shareToolCanonicalName(c.tool); got != c.want {
			t.Errorf("shareToolCanonicalName(%+v) = %q, want %q", c.tool, got, c.want)
		}
	}
}

func TestResolveTunnelTool_rejectsUnknownName(t *testing.T) {
	_, err := resolveTunnelTool("wireguard", "")
	if err == nil || !strings.Contains(err.Error(), "unknown tunnel tool") {
		t.Fatalf("expected unknown-tool error, got %v", err)
	}
}

func TestResolveTunnelTool_explicitNameIgnoresDefault(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "ngrok", "cloudflared"))
	tool, err := resolveTunnelTool("ngrok", "cloudflare")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.mode != shareModeNgrok {
		t.Errorf("mode = %v, want shareModeNgrok", tool.mode)
	}
}

// ── tunnel process lifecycle ──────────────────────────────────────────────────

func shellTunnel(script string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", script)
}

func waitTunnelGone(t *testing.T, key string) {
	t.Helper()
	for range 100 {
		if _, ok := tunnelStatusByKey(key); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tunnel for %s still reported after waiting", key)
}

func TestStartTunnelProcess_returnsURLAndTracksTunnel(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-a"
	cmd := shellTunnel(`echo "Forwarding HTTP traffic from https://abc123.serveo.net"; sleep 30`)
	url, err := startTunnelProcess(site, "serveo", cmd, nil, 5*time.Second, "")
	t.Cleanup(func() { _ = TunnelStop(site, "") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://abc123.serveo.net" {
		t.Errorf("url = %q, want the serveo URL", url)
	}
	info, ok := TunnelStatus(site, "")
	if !ok || info.URL != url || info.Tool != "serveo" {
		t.Errorf("TunnelStatus = %+v, %v; want running serveo tunnel", info, ok)
	}
	if err := TunnelStop(site, ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitTunnelGone(t, site)
}

func TestStartTunnelProcess_readsStderrToo(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-stderr"
	cmd := shellTunnel(`echo "INF |  https://plum-lakes-agree.trycloudflare.com  |" >&2; sleep 30`)
	url, err := startTunnelProcess(site, "cloudflare", cmd, nil, 5*time.Second, "")
	t.Cleanup(func() { _ = TunnelStop(site, "") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://plum-lakes-agree.trycloudflare.com" {
		t.Errorf("url = %q, want the trycloudflare URL", url)
	}
	_ = TunnelStop(site, "")
	waitTunnelGone(t, site)
}

func TestStartTunnelProcess_exitBeforeURLReportsOutput(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-fail"
	cmd := shellTunnel(`echo "ERR authentication failed: install your authtoken" >&2; exit 1`)
	_, err := startTunnelProcess(site, "ngrok", cmd, nil, 5*time.Second, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("error %q does not carry the tool output", err)
	}
	waitTunnelGone(t, site)
}

func TestStartTunnelProcess_timesOut(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-slow"
	cmd := shellTunnel(`sleep 30`)
	_, err := startTunnelProcess(site, "serveo", cmd, nil, 300*time.Millisecond, "")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	waitTunnelGone(t, site)
}

func TestStartTunnelProcess_replacesExistingTunnel(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-replace"
	t.Cleanup(func() { _ = TunnelStop(site, "") })
	first := shellTunnel(`echo "Forwarding HTTP traffic from https://first.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(site, "serveo", first, nil, 5*time.Second, ""); err != nil {
		t.Fatalf("first start: %v", err)
	}
	second := shellTunnel(`echo "Forwarding HTTP traffic from https://second.serveo.net"; sleep 30`)
	url, err := startTunnelProcess(site, "serveo", second, nil, 5*time.Second, "")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if url != "https://second.serveo.net" {
		t.Errorf("url = %q, want the second tunnel's URL", url)
	}
	info, ok := TunnelStatus(site, "")
	if !ok || info.URL != "https://second.serveo.net" {
		t.Errorf("TunnelStatus = %+v, %v; want the second tunnel", info, ok)
	}
}

func TestStartTunnelProcess_stopProxyRunsWhenProcessDies(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-proxy"
	stopped := make(chan struct{})
	cmd := shellTunnel(`echo "Forwarding HTTP traffic from https://p.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(site, "serveo", cmd, func() { close(stopped) }, 5*time.Second, ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := TunnelStop(site, ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stopProxy was not called after the tunnel stopped")
	}
}

func TestStopAllTunnels(t *testing.T) {
	tunnelStateHome(t)
	sites := []string{"tunnel-test-all-1", "tunnel-test-all-2"}
	for _, s := range sites {
		cmd := shellTunnel(`echo "Forwarding HTTP traffic from https://` + s + `.serveo.net"; sleep 30`)
		if _, err := startTunnelProcess(s, "serveo", cmd, nil, 5*time.Second, ""); err != nil {
			t.Fatalf("start %s: %v", s, err)
		}
	}
	StopAllTunnels()
	for _, s := range sites {
		waitTunnelGone(t, s)
	}
}

func TestTunnelStop_noopWhenNotRunning(t *testing.T) {
	tunnelStateHome(t)
	if err := TunnelStop("tunnel-test-none", ""); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
}

// ── worktree tunnels ──────────────────────────────────────────────────────────

func TestTunnelKey_separatesWorktreeFromSite(t *testing.T) {
	if got := tunnelKey("rapids", ""); got != "rapids" {
		t.Errorf("tunnelKey with no branch = %q, want the bare site name", got)
	}
	if tunnelKey("rapids", "feature") == tunnelKey("rapids", "") {
		t.Error("a worktree tunnel must not share the site's key")
	}
	if tunnelKey("rapids", "feature") == tunnelKey("rapids", "other") {
		t.Error("two branches of one site must not share a key")
	}
}

// A worktree fronts its own domain, so its tunnel is a separate process that
// neither replaces the parent's nor is stopped along with it.
func TestStartTunnelProcess_worktreeTunnelIsIndependentOfTheSite(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-wt"
	siteKey := tunnelKey(site, "")
	wtKey := tunnelKey(site, "feature")
	t.Cleanup(func() { _ = TunnelStop(site, ""); _ = TunnelStop(site, "feature") })

	parent := shellTunnel(`echo "Forwarding HTTP traffic from https://parent.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(siteKey, "serveo", parent, nil, 5*time.Second, ""); err != nil {
		t.Fatalf("parent start: %v", err)
	}
	wt := shellTunnel(`echo "Forwarding HTTP traffic from https://branch.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(wtKey, "serveo", wt, nil, 5*time.Second, ""); err != nil {
		t.Fatalf("worktree start: %v", err)
	}

	pInfo, ok := TunnelStatus(site, "")
	if !ok || pInfo.URL != "https://parent.serveo.net" {
		t.Errorf("site tunnel = %+v, %v; want the parent URL still running", pInfo, ok)
	}
	wInfo, ok := TunnelStatus(site, "feature")
	if !ok || wInfo.URL != "https://branch.serveo.net" {
		t.Errorf("worktree tunnel = %+v, %v; want the branch URL", wInfo, ok)
	}

	if err := TunnelStop(site, "feature"); err != nil {
		t.Fatalf("stop worktree: %v", err)
	}
	waitTunnelGone(t, wtKey)
	if _, ok := TunnelStatus(site, ""); !ok {
		t.Error("stopping the worktree tunnel also stopped the site's")
	}
}

// ── named Cloudflare tunnels ─────────────────────────────────────────────────

func TestTunnelURLFromLine_parsesTheOutputWhenNothingIsKnown(t *testing.T) {
	got, ok := tunnelURLFromLine("serveo", "", "Forwarding from https://abc.serveo.net")
	if !ok || got != "https://abc.serveo.net" {
		t.Errorf("got (%q, %v), want the serveo URL", got, ok)
	}
}

// A named tunnel prints no URL at all, so the scan waits for the line that says
// it is carrying traffic and reports the hostname it already knew.
func TestTunnelURLFromLine_knownURLWaitsForTheConnection(t *testing.T) {
	known := "https://acme.example.com"
	if _, ok := tunnelURLFromLine("cloudflare", known, "INF Starting tunnel tunnelID=abc"); ok {
		t.Error("the hostname was reported before the tunnel connected")
	}
	got, ok := tunnelURLFromLine("cloudflare", known, "INF Registered tunnel connection connIndex=0 protocol=quic")
	if !ok || got != known {
		t.Errorf("got (%q, %v), want %q once the connection registered", got, ok, known)
	}
}

func TestTunnelURLFromLine_knownURLAcceptsTheOlderWording(t *testing.T) {
	known := "https://acme.example.com"
	line := "INF Connection 0e8b1c4a-1111-2222-3333-444455556666 registered"
	if got, ok := tunnelURLFromLine("cloudflare", known, line); !ok || got != known {
		t.Errorf("got (%q, %v), want %q", got, ok, known)
	}
}

func TestStartTunnelProcess_namedTunnelReportsItsOwnHostname(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-named"
	known := "https://acme.example.com"
	cmd := shellTunnel(`echo "INF Registered tunnel connection connIndex=0"; sleep 30`)
	url, err := startTunnelProcess(site, "cloudflare", cmd, nil, 5*time.Second, known)
	t.Cleanup(func() { _ = TunnelStop(site, "") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != known {
		t.Errorf("url = %q, want %q", url, known)
	}
	if err := TunnelStop(site, ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitTunnelGone(t, site)
}

// A share started in a terminal is invisible to the daemon's own registry, so
// the status has to fall through to what the CLI recorded on disk.
func TestTunnelStatus_reportsAShareStartedFromTheCLI(t *testing.T) {
	tunnelStateHome(t)
	st := tunnelState{Site: "acme", Tool: "ngrok", URL: "https://abc.ngrok.io", PID: os.Getpid()}
	if err := writeTunnelState(tunnelKey("acme", ""), st); err != nil {
		t.Fatalf("writing: %v", err)
	}

	info, ok := TunnelStatus("acme", "")
	if !ok || info.URL != st.URL || !info.External {
		t.Errorf("TunnelStatus = %+v, %v; want the CLI tunnel marked external", info, ok)
	}
}

// The daemon's own tunnel is the live one; a leftover CLI entry for the same
// site must not shadow it.
func TestTunnelStatus_prefersTheDaemonsOwnTunnel(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-both"
	if err := writeTunnelState(tunnelKey(site, ""), tunnelState{
		Site: site, Tool: "ngrok", URL: "https://stale.ngrok.io", PID: os.Getpid(),
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	cmd := shellTunnel(`echo "Forwarding HTTP traffic from https://live.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(site, "serveo", cmd, nil, 5*time.Second, ""); err != nil {
		t.Fatalf("starting: %v", err)
	}
	t.Cleanup(func() { _ = TunnelStop(site, "") })

	info, ok := TunnelStatus(site, "")
	if !ok || info.URL != "https://live.serveo.net" || info.External {
		t.Errorf("TunnelStatus = %+v, %v; want the daemon's own tunnel", info, ok)
	}
}

// startSleeper runs a real process to stand in for a "lerd share" in a
// terminal, so a stop has something it can actually signal.
func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	c := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := c.Start(); err != nil {
		t.Fatalf("starting the stand-in share: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Process.Kill()
		_ = c.Wait()
	})
	return c
}

// A stop acts on the tunnel the dashboard is showing. With both kinds up that
// is the daemon's own, and the CLI share behind it takes over the display.
func TestTunnelStop_leavesTheCLIShareWhenWeOwnTheLiveTunnel(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-stop-both"
	sleeper := startSleeper(t)
	if err := writeTunnelState(tunnelKey(site, ""), tunnelState{
		Site: site, Tool: "ngrok", URL: "https://cli.ngrok.io", PID: sleeper.Process.Pid,
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	cmd := shellTunnel(`echo "Forwarding HTTP traffic from https://live.serveo.net"; sleep 30`)
	if _, err := startTunnelProcess(site, "serveo", cmd, nil, 5*time.Second, ""); err != nil {
		t.Fatalf("starting: %v", err)
	}
	t.Cleanup(func() { _ = TunnelStop(site, "") })

	if err := TunnelStop(site, ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	info, ok := TunnelStatus(site, "")
	if !ok || info.URL != "https://cli.ngrok.io" || !info.External {
		t.Errorf("after the stop TunnelStatus = %+v, %v; want the CLI share still up", info, ok)
	}
	if err := sleeper.Process.Signal(syscall.Signal(0)); err != nil {
		t.Error("the CLI share was signalled even though we owned the live tunnel")
	}
}

func TestTunnelStop_signalsACLIShareWhenItIsTheOnlyOne(t *testing.T) {
	tunnelStateHome(t)
	site := "tunnel-test-stop-cli"
	sleeper := startSleeper(t)
	if err := writeTunnelState(tunnelKey(site, ""), tunnelState{
		Site: site, Tool: "ngrok", URL: "https://cli.ngrok.io", PID: sleeper.Process.Pid,
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	if err := TunnelStop(site, ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- sleeper.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the CLI share was never signalled")
	}
}
