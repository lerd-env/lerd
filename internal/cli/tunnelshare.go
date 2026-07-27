package cli

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Public tunnels started from the UI. Unlike LAN share proxies these are
// deliberately not persisted: a tunnel dies with lerd-ui (systemd kills the
// control group, Pdeathsig covers non-systemd runs) and is never resurrected.

// TunnelInfo describes a running tunnel for the sites payload.
type TunnelInfo struct {
	Tool string `json:"tool"`
	URL  string `json:"url"`
}

type tunnelProc struct {
	tool      string
	url       string
	cmd       *exec.Cmd
	stopProxy func()
	done      chan struct{}
}

var (
	tunnelsMu sync.Mutex
	tunnels   = map[string]*tunnelProc{}
)

var tunnelStartTimeout = 45 * time.Second

// ShareToolStatus reports one tunnel tool and whether its binary is in PATH.
type ShareToolStatus struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Binary     string `json:"binary"`
	Installed  bool   `json:"installed"`
	InstallURL string `json:"install_url,omitempty"`
}

// ShareToolsInfo is the response for GET /api/share-tools.
type ShareToolsInfo struct {
	Tools   []ShareToolStatus `json:"tools"`
	Auto    string            `json:"auto,omitempty"`
	Default string            `json:"default,omitempty"`
}

var shareToolMeta = map[string]struct{ label, installURL string }{
	"ngrok":         {"ngrok", "https://ngrok.com/download"},
	"cloudflare":    {"Cloudflare Tunnel", "https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"},
	"expose":        {"Expose", "https://expose.dev"},
	"serveo":        {"Serveo", ""},
	"localhost-run": {"localhost.run", ""},
}

// ShareTools reports every supported tunnel tool, which ones are installed,
// and what the auto pick would use right now.
func ShareTools() ShareToolsInfo {
	defaultTool := ""
	if cfg, err := config.LoadGlobal(); err == nil {
		defaultTool = cfg.Share.DefaultTool
	}
	info := ShareToolsInfo{Default: defaultTool, Auto: autoShareToolName(defaultTool)}
	for _, t := range shareTools {
		meta := shareToolMeta[t.name]
		_, err := exec.LookPath(t.binary)
		info.Tools = append(info.Tools, ShareToolStatus{
			Name:       t.name,
			Label:      meta.label,
			Binary:     t.binary,
			Installed:  err == nil,
			InstallURL: meta.installURL,
		})
	}
	return info
}

// autoShareToolName mirrors pickShareTool's auto-detect order without its
// side effects, so the UI can display what a bare start would pick.
func autoShareToolName(defaultTool string) string {
	if bin, ok := shareToolBinary(defaultTool); ok {
		if _, err := exec.LookPath(bin); err == nil {
			return defaultTool
		}
	}
	for _, name := range []string{"ngrok", "cloudflare", "expose", "localhost-run"} {
		bin, _ := shareToolBinary(name)
		if _, err := exec.LookPath(bin); err == nil {
			return name
		}
	}
	return ""
}

// resolveTunnelTool maps a UI tool name to the shareTool the CLI flags would
// produce. Empty or "auto" runs the same auto-detection as a bare lerd share.
func resolveTunnelTool(name, defaultTool string) (*shareTool, error) {
	var ngrok, cloudflare, expose, serveo, localhostRun bool
	switch name {
	case "", "auto":
	case "ngrok":
		ngrok = true
	case "cloudflare":
		cloudflare = true
	case "expose":
		expose = true
	case "serveo":
		serveo = true
	case "localhost-run":
		localhostRun = true
	default:
		return nil, fmt.Errorf("unknown tunnel tool %q: use %s, or auto", name, strings.Join(shareToolNames(), ", "))
	}
	if name != "" && name != "auto" {
		defaultTool = ""
	}
	return pickShareTool(ngrok, cloudflare, expose, serveo, localhostRun, "", defaultTool)
}

func shareToolCanonicalName(t *shareTool) string {
	switch t.mode {
	case shareModeNgrok:
		return "ngrok"
	case shareModeCloudflare:
		return "cloudflare"
	case shareModeExpose:
		return "expose"
	case shareModeSSH:
		if t.sshHost == "serveo.net" {
			return "serveo"
		}
		return "localhost-run"
	}
	return ""
}

var tunnelURLPatterns = map[string][]*regexp.Regexp{
	"ngrok":      {regexp.MustCompile(`"url":"(https://[^"]+)"`)},
	"cloudflare": {regexp.MustCompile(`(https://[a-z0-9-]+\.trycloudflare\.com)`)},
	"expose": {
		regexp.MustCompile(`(?i)(?:expose-url|public https):\s*(https?://\S+)`),
		regexp.MustCompile(`(https://[a-zA-Z0-9.-]+\.sharedwithexpose\.com\S*)`),
	},
	"serveo":        {regexp.MustCompile(`(https://[a-zA-Z0-9.-]+\.serveo\.net\S*)`)},
	"localhost-run": {regexp.MustCompile(`(https://[a-z0-9-]+\.lhr\.life\S*)`)},
}

// parseTunnelURL extracts the public URL from one line of tool output.
func parseTunnelURL(tool, line string) (string, bool) {
	for _, re := range tunnelURLPatterns[tool] {
		if m := re.FindStringSubmatch(line); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// tunnelKey identifies a running tunnel. A worktree fronts its own domain, so
// it gets a key of its own rather than replacing the parent site's tunnel. The
// bare site name is kept as the key for the site itself.
func tunnelKey(siteName, branch string) string {
	if branch == "" {
		return siteName
	}
	return siteName + "@" + branch
}

// tunnelStatusByKey returns the running tunnel registered under key.
func tunnelStatusByKey(key string) (TunnelInfo, bool) {
	tunnelsMu.Lock()
	defer tunnelsMu.Unlock()
	p := tunnels[key]
	if p == nil || p.url == "" {
		return TunnelInfo{}, false
	}
	return TunnelInfo{Tool: p.tool, URL: p.url}, true
}

// TunnelStatus returns the running tunnel for a site, or for one of its
// worktrees when branch is set, if one has a URL yet.
func TunnelStatus(siteName, branch string) (TunnelInfo, bool) {
	return tunnelStatusByKey(tunnelKey(siteName, branch))
}

// TunnelStart starts a public tunnel for the site, or for one of its worktrees
// when branch is set, and blocks until the tool prints its public URL. An empty
// toolName auto-picks like the CLI does.
func TunnelStart(siteName, branch, toolName string) (string, error) {
	site, err := config.FindSite(siteName)
	if err != nil {
		return "", err
	}
	target, err := shareTargetFor(site, branch)
	if err != nil {
		return "", err
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	httpPort := cfg.Nginx.HTTPPort
	if httpPort == 0 {
		httpPort = 80
	}
	httpsPort := cfg.Nginx.HTTPSPort
	if httpsPort == 0 {
		httpsPort = 443
	}
	tool, err := resolveTunnelTool(toolName, cfg.Share.DefaultTool)
	if err != nil {
		return "", err
	}
	cmd, stopProxy, err := buildTunnelCommand(tool, "", target, httpPort, httpsPort, true)
	if err != nil {
		return "", err
	}
	return startTunnelProcess(tunnelKey(siteName, branch), shareToolCanonicalName(tool), cmd, stopProxy, tunnelStartTimeout)
}

// startTunnelProcess runs the tool, scans its output for the public URL, and
// registers the tunnel under key. It replaces any tunnel already running for
// that key. After a successful Start the exit watcher owns stopProxy.
func startTunnelProcess(key, toolName string, cmd *exec.Cmd, stopProxy func(), timeout time.Duration) (string, error) {
	stopTunnelByKey(key)
	if stopProxy == nil {
		stopProxy = func() {}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stopProxy()
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stopProxy()
		return "", err
	}
	cmd.SysProcAttr = tunnelSysProcAttr()
	if err := cmd.Start(); err != nil {
		stopProxy()
		return "", err
	}

	p := &tunnelProc{tool: toolName, cmd: cmd, stopProxy: stopProxy, done: make(chan struct{})}
	tunnelsMu.Lock()
	tunnels[key] = p
	tunnelsMu.Unlock()

	urlCh := make(chan string, 1)
	var tailMu sync.Mutex
	var tail []string
	var readers sync.WaitGroup
	scan := func(r io.Reader) {
		defer readers.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
		for sc.Scan() {
			line := sc.Text()
			tailMu.Lock()
			if tail = append(tail, line); len(tail) > 12 {
				tail = tail[1:]
			}
			tailMu.Unlock()
			if u, ok := parseTunnelURL(toolName, line); ok {
				select {
				case urlCh <- u:
				default:
				}
			}
		}
	}
	readers.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		readers.Wait()
		_ = cmd.Wait()
		stopProxy()
		close(p.done)
		tunnelsMu.Lock()
		if tunnels[key] == p {
			delete(tunnels, key)
		}
		tunnelsMu.Unlock()
	}()

	select {
	case u := <-urlCh:
		tunnelsMu.Lock()
		p.url = u
		tunnelsMu.Unlock()
		return u, nil
	case <-p.done:
		tailMu.Lock()
		out := strings.TrimSpace(strings.Join(tail, "\n"))
		tailMu.Unlock()
		return "", fmt.Errorf("%s exited before printing a public URL: %s", toolName, out)
	case <-time.After(timeout):
		tunnelsMu.Lock()
		if tunnels[key] == p {
			delete(tunnels, key)
		}
		tunnelsMu.Unlock()
		killTunnel(p)
		return "", fmt.Errorf("timed out waiting for %s to print a public URL", toolName)
	}
}

// TunnelStop stops the tunnel for a site, or for one of its worktrees when
// branch is set. A no-op when none is running.
func TunnelStop(siteName, branch string) error {
	stopTunnelByKey(tunnelKey(siteName, branch))
	return nil
}

// stopTunnelByKey kills the tunnel registered under key, if any.
func stopTunnelByKey(key string) {
	tunnelsMu.Lock()
	p := tunnels[key]
	delete(tunnels, key)
	tunnelsMu.Unlock()
	if p == nil {
		return
	}
	killTunnel(p)
}

// StopAllTunnels kills every running tunnel.
func StopAllTunnels() {
	tunnelsMu.Lock()
	procs := make([]*tunnelProc, 0, len(tunnels))
	for name, p := range tunnels {
		procs = append(procs, p)
		delete(tunnels, name)
	}
	tunnelsMu.Unlock()
	for _, p := range procs {
		killTunnel(p)
	}
}

// killTunnel terminates the whole process group (ssh and cloudflared may have
// children) and escalates to SIGKILL if it lingers.
func killTunnel(p *tunnelProc) {
	if p.cmd.Process == nil {
		return
	}
	pgid := -p.cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	select {
	case <-p.done:
		return
	case <-time.After(3 * time.Second):
	}
	_ = syscall.Kill(pgid, syscall.SIGKILL)
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
	}
}
