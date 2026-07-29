package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
)

// Public tunnels started from the UI. Unlike LAN share proxies these are
// deliberately not persisted: a tunnel dies with lerd-ui and is never
// resurrected. Systemd kills the control group and Pdeathsig covers a
// non-systemd run; on macOS neither applies, so StopAllTunnels runs from the
// shutdown handler and anything that outlived a SIGKILL is reaped at the next
// start (see tunnel_reap.go).

// TunnelInfo describes a running tunnel for the sites payload. External marks
// one started by "lerd share" in a terminal rather than from the dashboard.
type TunnelInfo struct {
	Tool     string `json:"tool"`
	URL      string `json:"url"`
	External bool   `json:"external,omitempty"`
}

type tunnelProc struct {
	tool      string
	url       string
	cmd       *exec.Cmd
	stopProxy func()
	done      chan struct{}
	// container names the tunnel's container when the tool runs as one. Killing
	// the podman client does not stop it, so the name is what tears it down.
	container string
}

var (
	tunnelsMu sync.Mutex
	tunnels   = map[string]*tunnelProc{}
)

var tunnelStartTimeout = 45 * time.Second

// ShareToolStatus reports one tunnel tool and whether its binary is in PATH.
type ShareToolStatus struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Binary    string `json:"binary"`
	Installed bool   `json:"installed"`
	// Containerised says the tool has no binary here but runs from a published
	// image instead, so the menu offers it rather than sending the user to an
	// install page they do not need.
	Containerised bool `json:"containerised,omitempty"`
	// NeedsToken says the image is the only route left and it cannot run
	// without a token, so the menu points at the token rather than at an
	// install page the user does not have to visit.
	NeedsToken bool   `json:"needs_token,omitempty"`
	InstallURL string `json:"install_url,omitempty"`
}

// ShareToolsInfo is the response for GET /api/share-tools.
type ShareToolsInfo struct {
	Tools   []ShareToolStatus `json:"tools"`
	Auto    string            `json:"auto,omitempty"`
	Default string            `json:"default,omitempty"`
	// BaseDomain is the Cloudflare-managed domain a share is served under, and
	// BaseDomainAnswered says whether that answer was remembered, so the share
	// menu knows whether it still has to ask.
	BaseDomain         string `json:"base_domain,omitempty"`
	BaseDomainAnswered bool   `json:"base_domain_answered,omitempty"`
	// NgrokTokenSet reports only whether a token is stored. The token itself is
	// a credential and never leaves the machine through this endpoint.
	NgrokTokenSet bool `json:"ngrok_token_set,omitempty"`
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
	defaultTool, baseDomain, answered, ngrokToken := "", "", false, ""
	if cfg, err := config.LoadGlobal(); err == nil {
		defaultTool = cfg.Share.DefaultTool
		baseDomain, answered = cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered
		ngrokToken = cfg.Share.NgrokToken
	}
	info := ShareToolsInfo{
		Default:            defaultTool,
		Auto:               autoShareToolName(defaultTool, ngrokToken),
		BaseDomain:         baseDomain,
		BaseDomainAnswered: answered,
		NgrokTokenSet:      ngrokToken != "",
	}
	for _, t := range shareTools {
		meta := shareToolMeta[t.name]
		_, found := hostbin.Look(t.binary)
		// ngrok is the one tool with a published image, so a missing binary is
		// only a dead end when there is no token to run the image with.
		image := !found && t.name == "ngrok"
		info.Tools = append(info.Tools, ShareToolStatus{
			Name:          t.name,
			Label:         meta.label,
			Binary:        t.binary,
			Installed:     found || (image && ngrokToken != ""),
			Containerised: image && ngrokToken != "",
			NeedsToken:    image && ngrokToken == "",
			InstallURL:    meta.installURL,
		})
	}
	return info
}

// autoShareToolName mirrors pickShareTool's auto-detect order without its
// side effects, so the UI can display what a bare start would pick.
func autoShareToolName(defaultTool, ngrokToken string) string {
	if bin, ok := shareToolBinary(defaultTool); ok {
		if _, found := hostbin.Look(bin); found {
			return defaultTool
		}
	}
	if defaultTool == "ngrok" && ngrokToken != "" {
		return "ngrok"
	}
	for _, name := range []string{"ngrok", "cloudflare", "expose"} {
		bin, _ := shareToolBinary(name)
		if _, found := hostbin.Look(bin); found {
			return name
		}
	}
	// Mirrors pickShareTool: with nothing installed, a stored token puts ngrok
	// ahead of the SSH fallback.
	if ngrokToken != "" {
		return "ngrok"
	}
	if _, found := hostbin.Look("ssh"); found {
		return "localhost-run"
	}
	return ""
}

// resolveTunnelTool maps a UI tool name to the shareTool the CLI flags would
// produce. Empty or "auto" runs the same auto-detection as a bare lerd share.
func resolveTunnelTool(name, defaultTool, ngrokToken string) (*shareTool, error) {
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
	return pickShareTool(ngrok, cloudflare, expose, serveo, localhostRun, "", defaultTool, ngrokToken)
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

// cloudflareConnected matches the line a named tunnel logs once it is carrying
// traffic. Both spellings cloudflared has used are accepted.
var cloudflareConnected = regexp.MustCompile(`(?i)(registered tunnel connection|connection [0-9a-f-]+ registered)`)

// tunnelURLFromLine reports the public URL a line of output announces. A named
// Cloudflare tunnel serves a hostname that is known before it starts, so there
// the scan waits for the connection to register and reports that hostname.
func tunnelURLFromLine(tool, known, line string) (string, bool) {
	if known != "" {
		return known, cloudflareConnected.MatchString(line)
	}
	return parseTunnelURL(tool, line)
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

// tunnelStatusByKey returns the running tunnel registered under key, falling
// back to one a "lerd share" in a terminal recorded for us.
func tunnelStatusByKey(key string) (TunnelInfo, bool) {
	tunnelsMu.Lock()
	p := tunnels[key]
	tunnelsMu.Unlock()
	if p != nil && p.url != "" {
		return TunnelInfo{Tool: p.tool, URL: p.url}, true
	}
	return cliTunnelStatus(key)
}

// TunnelStatus returns the running tunnel for a site, or for one of its
// worktrees when branch is set, if one has a URL yet.
func TunnelStatus(siteName, branch string) (TunnelInfo, bool) {
	return tunnelStatusByKey(tunnelKey(siteName, branch))
}

// resolveShareBaseDomain settles which base domain a start serves under. One
// given for this run only works with Cloudflare Tunnel, the same way --domain
// does; the configured one simply does not apply to the other tools.
func resolveShareBaseDomain(tool *shareTool, requested, configured string) (string, error) {
	d, err := normalizeBaseDomain(requested)
	if err != nil {
		return "", err
	}
	if tool.mode != shareModeCloudflare {
		if d != "" {
			return "", fmt.Errorf("a base domain only works with Cloudflare Tunnel, pick that tool instead")
		}
		return "", nil
	}
	if d == "" {
		d = configured
	}
	return d, nil
}

// SetShareBaseDomain records the answer to the base-domain question. remember
// false forgets it, so the next share asks again.
func SetShareBaseDomain(baseDomain string, remember bool) error {
	domain, err := normalizeBaseDomain(baseDomain)
	if err != nil {
		return err
	}
	if !remember {
		domain = ""
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if cfg.Share.BaseDomain == domain && cfg.Share.BaseDomainAnswered == remember {
		return nil
	}
	cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered = domain, remember
	return config.SaveGlobal(cfg)
}

// SetShareNgrokToken stores the ngrok auth token, or clears it when empty. The
// config file holds a credential once this is set, so it is written back with
// owner-only permissions.
func SetShareNgrokToken(token string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if cfg.Share.NgrokToken == token {
		return nil
	}
	cfg.Share.NgrokToken = token
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	// WriteFile keeps an existing file's mode, so tightening it once holds for
	// every later save rather than being widened back to the default.
	return os.Chmod(config.GlobalConfigFile(), 0600)
}

// TunnelStart starts a public tunnel for the site, or for one of its worktrees
// when branch is set, and blocks until the tool reports its public URL. An empty
// toolName auto-picks like the CLI does. baseDomain serves the site on
// "<site>.<base domain>" through a Cloudflare named tunnel for this run;
// empty falls back to the configured one.
func TunnelStart(siteName, branch, toolName, baseDomain string) (string, error) {
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
	tool, err := resolveTunnelTool(toolName, cfg.Share.DefaultTool, cfg.Share.NgrokToken)
	if err != nil {
		return "", err
	}
	base, err := resolveShareBaseDomain(tool, baseDomain, cfg.Share.BaseDomain)
	if err != nil {
		return "", err
	}

	// A named tunnel serves a hostname known before it starts, so the scan has
	// nothing to parse out of the output and waits for the connection instead.
	tunnelName, known := "", ""
	if base != "" {
		hostname, hErr := shareHostname(target.domain, cfg.DNS.TLD, base)
		if hErr != nil {
			return "", hErr
		}
		if tunnelName, err = ensureCloudflareTunnel(target.name, hostname, false); err != nil {
			return "", err
		}
		known = "https://" + hostname
	}

	cmd, stopProxy, err := buildTunnelCommand(tool, tunnelName, target, httpPort, httpsPort, true)
	if err != nil {
		return "", err
	}
	container := ""
	if tool.mode == shareModeNgrok && tool.ngrok.container {
		container = ngrokContainerName(target.name, "")
	}
	return startTunnelProcess(tunnelKey(siteName, branch), shareToolCanonicalName(tool), cmd, stopProxy, tunnelStartTimeout, known, container)
}

// startTunnelProcess runs the tool, scans its output for the public URL, and
// registers the tunnel under key. It replaces any tunnel already running for
// that key. After a successful Start the exit watcher owns stopProxy.
func startTunnelProcess(key, toolName string, cmd *exec.Cmd, stopProxy func(), timeout time.Duration, known, container string) (string, error) {
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

	p := &tunnelProc{tool: toolName, cmd: cmd, stopProxy: stopProxy, done: make(chan struct{}), container: container}
	tunnelsMu.Lock()
	tunnels[key] = p
	tunnelsMu.Unlock()
	recordTunnel(cmd.Process.Pid, cmd.Args)

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
			if u, ok := tunnelURLFromLine(toolName, known, line); ok {
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
		forgetTunnel(cmd.Process.Pid)
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
// branch is set, wherever it was started from. A no-op when none is running.
// Our own tunnel is the one the dashboard is showing when both exist, so that
// is the one a stop acts on; the CLI share behind it stays up and takes over
// the display.
func TunnelStop(siteName, branch string) error {
	key := tunnelKey(siteName, branch)
	if !stopTunnelByKey(key) {
		stopCLITunnel(key)
	}
	return nil
}

// stopTunnelByKey kills the tunnel registered under key and reports whether
// there was one.
func stopTunnelByKey(key string) bool {
	tunnelsMu.Lock()
	p := tunnels[key]
	delete(tunnels, key)
	tunnelsMu.Unlock()
	if p == nil {
		return false
	}
	killTunnel(p)
	return true
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
	// The container is removed either way: the escalation below ends with a
	// SIGKILL the client cannot forward, which would leave the tunnel serving.
	defer removeNgrokContainer(p.container)
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
