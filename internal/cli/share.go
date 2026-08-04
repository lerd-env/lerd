package cli

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/hostbin"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewShareCmd returns the share command.
func NewShareCmd() *cobra.Command {
	var useNgrok bool
	var useCloudflare bool
	var useExpose bool
	var useServeo bool
	var useLocalhostRun bool
	var usePinggy bool
	var domain string
	var token string

	cmd := &cobra.Command{
		Use:   "share [site]",
		Short: "Expose the current site publicly via a tunnel",
		Long: `Starts a public tunnel to the current site.

Auto-detection order: ngrok → Cloudflare Tunnel → Expose → localhost.run (SSH, no signup).

Supported tools:
  ngrok          https://ngrok.com/download
  cloudflared    https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/
  expose         https://expose.dev
  localhost.run  free SSH tunnel, no account needed (--localhost-run)
  serveo.net     free SSH tunnel, no account needed (--serveo)
  Pinggy         free SSH tunnel, no account needed (--pinggy); a token from
                 "lerd share:token pinggy" gives a stable subdomain

A default tool can be set with "lerd share:tool"; flags override it per run.

--domain selects Cloudflare Tunnel on its own: a named tunnel is created (or
reused) and the given hostname is routed to it, so the site is served on your
own domain instead of a random trycloudflare.com URL. The domain's DNS must be
managed by Cloudflare.

A base domain set with "lerd share:domain" does the same without the flag: a
Cloudflare share is served on "<site>.<base domain>".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runShare(args, useNgrok, useCloudflare, useExpose, useServeo, useLocalhostRun, usePinggy, domain, token)
		},
	}

	cmd.Flags().BoolVar(&useNgrok, "ngrok", false, "Use ngrok")
	cmd.Flags().BoolVar(&useCloudflare, "cloudflare", false, "Use Cloudflare Tunnel (cloudflared)")
	cmd.Flags().BoolVar(&useExpose, "expose", false, "Use Expose")
	cmd.Flags().BoolVar(&useServeo, "serveo", false, "Use serveo.net (SSH, no signup)")
	cmd.Flags().BoolVar(&useLocalhostRun, "localhost-run", false, "Use localhost.run (SSH, no signup)")
	cmd.Flags().BoolVar(&usePinggy, "pinggy", false, "Use Pinggy (SSH, no signup)")
	cmd.Flags().StringVar(&domain, "domain", "", "Serve on your own Cloudflare-managed hostname (implies Cloudflare Tunnel)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token for this run, overriding the stored one (ngrok, or Pinggy with --pinggy)")
	return cmd
}

type shareMode int

const (
	shareModeNgrok      shareMode = iota
	shareModeExpose     shareMode = iota
	shareModeSSH        shareMode = iota
	shareModeCloudflare shareMode = iota
)

type shareTool struct {
	mode   shareMode
	ssh    sshProvider // only for shareModeSSH
	domain string      // only for shareModeCloudflare: user's own hostname (named tunnel)
	// ngrok / ngrokToken are only for shareModeNgrok: which route runs the
	// tool, and the token that authenticates it on either of them.
	ngrok      ngrokRunner
	ngrokToken string
}

// sshProvider is the connection shape of one SSH tunnel provider. The providers
// disagree on every part of the invocation, so the mode carries the whole shape
// rather than just a host.
type sshProvider struct {
	name string // canonical tool name
	host string
	port int    // 0 uses ssh's default port
	user string // "" connects without a user part
	// remotePort is the remote side of the -R forward; "0" asks the server to
	// pick one, which is how Pinggy allocates a tunnel.
	remotePort string
}

var sshProviders = map[string]sshProvider{
	"serveo":        {name: "serveo", host: "serveo.net", user: "nokey", remotePort: "80"},
	"localhost-run": {name: "localhost-run", host: "localhost.run", user: "nokey", remotePort: "80"},
	"pinggy":        {name: "pinggy", host: "free.pinggy.io", port: 443, remotePort: "0"},
}

// pinggyProHost serves tokened Pinggy tunnels; the token rides as the SSH user.
const pinggyProHost = "pro.pinggy.io"

// pinggySSHProvider is the pinggy entry with the stored token applied: a token
// moves the connection to the pro endpoint and becomes the SSH user.
func pinggySSHProvider(token string) sshProvider {
	p := sshProviders["pinggy"]
	if token != "" {
		p.host, p.user = pinggyProHost, token
	}
	return p
}

func runShare(args []string, useNgrok, useCloudflare, useExpose, useServeo, useLocalhostRun, usePinggy bool, domain, token string) error {
	site, branch, err := resolveShareSite(args)
	if err != nil {
		return err
	}
	target, err := shareTargetFor(site, branch)
	if err != nil {
		return err
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	port := cfg.Nginx.HTTPPort
	if port == 0 {
		port = 80
	}

	// The flag is for a single run, so it outranks the stored token without
	// replacing it. It goes to Pinggy when Pinggy was picked explicitly, and
	// keeps meaning ngrok everywhere else.
	ngrokToken := cfg.Share.NgrokToken
	pinggyToken := cfg.Share.PinggyToken
	if token != "" {
		if usePinggy {
			pinggyToken = token
		} else {
			ngrokToken = token
		}
	}

	tool, err := pickShareTool(useNgrok, useCloudflare, useExpose, useServeo, useLocalhostRun, usePinggy, domain, cfg.Share.DefaultTool, ngrokToken, pinggyToken)
	if err != nil {
		return err
	}

	// A configured base domain serves the site on "<site>.<base>" without the
	// flag, so a bare share matches what the dashboard does.
	if tool.mode == shareModeCloudflare && tool.domain == "" && cfg.Share.BaseDomain != "" {
		if tool.domain, err = shareHostname(target.domain, cfg.DNS.TLD, cfg.Share.BaseDomain); err != nil {
			return err
		}
	}

	fmt.Printf("Sharing %s...\n", target.domain)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	tunnelName := ""
	if tool.mode == shareModeCloudflare && tool.domain != "" {
		if tunnelName, err = ensureCloudflareTunnel(target.name, tool.domain, true); err != nil {
			return err
		}
		fmt.Printf("Public URL: https://%s\n\n", tool.domain)
	}

	httpsPort := cfg.Nginx.HTTPSPort
	if httpsPort == 0 {
		httpsPort = 443
	}
	cmd, stop, err := buildTunnelCommand(tool, tunnelName, target, port, httpsPort, false)
	if err != nil {
		return err
	}
	defer stop()

	// Record the share so the dashboard shows it like one of its own, teeing
	// the tool's output through the recorder to catch the URL it announces.
	key := tunnelKey(site.Name, branch)
	state := tunnelState{Site: site.Name, Branch: branch, Tool: shareToolCanonicalName(tool), PID: os.Getpid()}
	if tool.domain != "" {
		state.URL = "https://" + tool.domain
	}
	recorder := newTunnelStateWriter(key, state)
	defer recorder.clear()

	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, recorder)
	cmd.Stderr = io.MultiWriter(os.Stderr, recorder)

	// The dashboard stops a CLI share by signalling this process, which the
	// tool itself never sees, so pass it on and let the run end normally.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	done := make(chan struct{})
	defer close(done)
	var signalled atomic.Bool
	go func() {
		select {
		case <-sigs:
			signalled.Store(true)
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
		case <-done:
		}
	}()

	if err := cmd.Run(); err != nil && !signalled.Load() {
		return err
	}
	return nil
}

// shareTarget is what a tunnel fronts: the domain nginx routes on and whether
// that vhost is served over HTTPS. A worktree resolves to its own subdomain
// under the parent's TLS state, the same way the LAN share proxy does.
type shareTarget struct {
	// name identifies the target when a tool needs a stable handle for it,
	// which today is the Cloudflare named tunnel.
	name    string
	domain  string
	secured bool
}

// shareTargetFor resolves a site, and optionally one of its worktree branches,
// to the domain a tunnel should front.
func shareTargetFor(site *config.Site, branch string) (shareTarget, error) {
	domain, err := siteops.WorktreeDomain(site, branch)
	if err != nil {
		return shareTarget{}, err
	}
	name := site.Name
	if branch != "" {
		name = site.Name + "-" + gitpkg.SanitizeBranch(branch)
	}
	return shareTarget{name: name, domain: domain, secured: site.Secured}, nil
}

// normalizeBaseDomain cleans up what a user typed for a Cloudflare base domain
// and rejects anything that is not a bare registrable domain. Empty stays
// empty: that means "no base domain", not a mistake.
func normalizeBaseDomain(s string) (string, error) {
	d := strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".")
	if d == "" {
		return "", nil
	}
	if strings.ContainsAny(d, "/:@ \t") || strings.HasPrefix(d, "*") {
		return "", fmt.Errorf("base domain %q must be a bare domain like example.com, without a scheme, port, path or wildcard", s)
	}
	if !strings.Contains(d, ".") {
		return "", fmt.Errorf("base domain %q needs at least one dot, like example.com", s)
	}
	for _, label := range strings.Split(d, ".") {
		if !isDNSLabel(label) {
			return "", fmt.Errorf("base domain %q is not a valid domain name", s)
		}
	}
	return d, nil
}

// isDNSLabel reports whether s is usable as one label of a hostname.
func isDNSLabel(s string) bool {
	if s == "" || len(s) > 63 || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// shareHostLabel is the single label a site is served under on a base domain.
// It comes from the domain the user picked, not from the folder the project
// happens to sit in, so scorediviner.test is shared as scorediviner. A
// worktree's subdomain flattens into that same label, because a Cloudflare
// certificate covers one level of subdomain and no more.
func shareHostLabel(domain, tld string) string {
	d := strings.Trim(strings.ToLower(domain), ".")
	if tld != "" && strings.HasSuffix(d, "."+tld) {
		d = strings.TrimSuffix(d, "."+tld)
	} else if i := strings.LastIndex(d, "."); i > 0 {
		d = d[:i]
	}
	return strings.ReplaceAll(d, ".", "-")
}

// shareHostname composes the public hostname a site's domain is served on under
// a base domain.
func shareHostname(domain, tld, baseDomain string) (string, error) {
	base, err := normalizeBaseDomain(baseDomain)
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", nil
	}
	label := shareHostLabel(domain, tld)
	if !isDNSLabel(label) {
		return "", fmt.Errorf("domain %q cannot be used as a hostname label under %s", domain, base)
	}
	return label + "." + base, nil
}

// buildTunnelCommand builds the tunnel tool invocation for a target plus a stop
// func for the local helper proxy the cloudflare/ssh modes need. headless
// swaps interactive output for parseable logs so a caller can scrape the
// public URL.
func buildTunnelCommand(tool *shareTool, tunnelName string, target shareTarget, httpPort, httpsPort int, headless bool) (*exec.Cmd, func(), error) {
	stop := func() {}
	var cmd *exec.Cmd
	switch tool.mode {
	case shareModeNgrok:
		proxyPort, stopProxy, err := startHostProxy(target.domain, httpPort, httpsPort, target.secured)
		if err != nil {
			return nil, nil, fmt.Errorf("starting local proxy: %w", err)
		}
		name := ngrokContainerName(target.name, "")
		stop = stopProxy
		if tool.ngrok.container {
			// The container outlives the podman client, so tearing the tunnel
			// down has to remove it by name rather than signal a process.
			stop = func() { removeNgrokContainer(name); stopProxy() }
		}
		cmd = tool.ngrok.cmd(proxyPort, tool.ngrokToken, headless, name)
	case shareModeExpose:
		proxyPort, stopProxy, err := startHostProxy(target.domain, httpPort, httpsPort, target.secured)
		if err != nil {
			return nil, nil, fmt.Errorf("starting local proxy: %w", err)
		}
		stop = stopProxy
		cmd = exec.Command(hostbin.Path("expose"), "share", fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	case shareModeCloudflare:
		proxyPort, stopProxy, err := startHostProxy(target.domain, httpPort, httpsPort, target.secured)
		if err != nil {
			return nil, nil, fmt.Errorf("starting local proxy: %w", err)
		}
		stop = stopProxy
		if tunnelName != "" {
			cmd = exec.Command(hostbin.Path("cloudflared"), "tunnel", "run",
				"--url", fmt.Sprintf("http://127.0.0.1:%d", proxyPort), tunnelName)
		} else {
			cmd = exec.Command(hostbin.Path("cloudflared"), "tunnel",
				"--url", fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		}
	case shareModeSSH:
		// Start a local reverse proxy that rewrites Host so nginx routes correctly.
		proxyPort, stopProxy, err := startHostProxy(target.domain, httpPort, httpsPort, target.secured)
		if err != nil {
			return nil, nil, fmt.Errorf("starting local proxy: %w", err)
		}
		stop = stopProxy
		if !headless {
			fmt.Printf("Local proxy started on port %d (Host: %s → nginx:%d)\n\n", proxyPort, target.domain, httpPort)
		}
		args := []string{
			"-o", "StrictHostKeyChecking=no",
			"-o", "ServerAliveInterval=30",
		}
		if tool.ssh.port != 0 {
			args = append(args, "-p", strconv.Itoa(tool.ssh.port))
		}
		dest := tool.ssh.host
		if tool.ssh.user != "" {
			dest = tool.ssh.user + "@" + dest
		}
		args = append(args, "-R", fmt.Sprintf("%s:localhost:%d", tool.ssh.remotePort, proxyPort), dest)
		cmd = exec.Command(hostbin.Path("ssh"), args...)
	default:
		// Unreachable through pickShareTool, but a nil command here would be a
		// nil dereference in the caller rather than an error it can report.
		stop()
		return nil, nil, fmt.Errorf("unsupported share tool")
	}
	return cmd, stop, nil
}

// resolveShareSite finds the site for the given name arg (or CWD if no arg),
// plus the worktree branch when the CWD is one. Naming a site explicitly always
// means the site itself, never a branch of it.
func resolveShareSite(args []string) (*config.Site, string, error) {
	if len(args) == 1 {
		site, err := config.FindSite(args[0])
		if err != nil {
			return nil, "", fmt.Errorf("site %q not found — run 'lerd sites' to list registered sites", args[0])
		}
		return site, "", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}

	site, err := config.FindSiteByPath(cwd)
	if err == nil {
		return site, "", nil
	}
	if parent, branch, ok := findOwningWorktree(cwd); ok {
		return parent, branch, nil
	}

	// Fall back: treat the directory name as a site name.
	name, _ := siteops.SiteNameAndDomain(filepath.Base(cwd), "test")
	if site, err = config.FindSite(name); err == nil {
		return site, "", nil
	}
	return ensureSiteAndBranchForCwd()
}

func pickShareTool(useNgrok, useCloudflare, useExpose, useServeo, useLocalhostRun, usePinggy bool, domain, defaultTool, ngrokToken, pinggyToken string) (*shareTool, error) {
	count := 0
	for _, f := range []bool{useNgrok, useCloudflare, useExpose, useServeo, useLocalhostRun, usePinggy} {
		if f {
			count++
		}
	}
	if count > 1 {
		return nil, fmt.Errorf("only one of --ngrok, --cloudflare, --expose, --serveo, --localhost-run, --pinggy may be specified")
	}
	fromDefault := false

	// --domain only means anything for Cloudflare Tunnel, so it picks the tool
	// itself and outranks the configured default. Another explicit tool flag is
	// a real conflict, so say so instead of silently overriding it.
	if domain != "" {
		if count > 0 && !useCloudflare {
			return nil, fmt.Errorf("--domain only works with Cloudflare Tunnel, drop the other tool flag")
		}
		useCloudflare = true
	} else if count == 0 && defaultTool != "" {
		// No tool flag: fall back to the configured default before auto-detecting.
		fromDefault = true
		switch defaultTool {
		case "ngrok":
			useNgrok = true
		case "cloudflare":
			useCloudflare = true
		case "expose":
			useExpose = true
		case "serveo":
			useServeo = true
		case "localhost-run":
			useLocalhostRun = true
		case "pinggy":
			usePinggy = true
		default:
			return nil, fmt.Errorf("unknown default share tool %q in config: run \"lerd share:tool\" to fix it", defaultTool)
		}
	}

	if useNgrok {
		runner, err := ngrokRunnerFor(ngrokToken)
		if err != nil {
			return nil, fmt.Errorf("%w%s", err, defaultToolHint(fromDefault))
		}
		return &shareTool{mode: shareModeNgrok, ngrok: runner, ngrokToken: ngrokToken}, nil
	}
	if useCloudflare {
		if _, ok := hostbin.Look("cloudflared"); !ok {
			return nil, fmt.Errorf("cloudflared not found — install it from https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/%s", defaultToolHint(fromDefault))
		}
		return &shareTool{mode: shareModeCloudflare, domain: domain}, nil
	}
	if useExpose {
		if _, ok := hostbin.Look("expose"); !ok {
			return nil, fmt.Errorf("expose not found — install it from https://expose.dev%s", defaultToolHint(fromDefault))
		}
		return &shareTool{mode: shareModeExpose}, nil
	}
	if useServeo {
		return &shareTool{mode: shareModeSSH, ssh: sshProviders["serveo"]}, nil
	}
	if useLocalhostRun {
		return &shareTool{mode: shareModeSSH, ssh: sshProviders["localhost-run"]}, nil
	}
	if usePinggy {
		return &shareTool{mode: shareModeSSH, ssh: pinggySSHProvider(pinggyToken)}, nil
	}

	// Auto-detect.
	if _, ok := hostbin.Look("ngrok"); ok {
		return &shareTool{mode: shareModeNgrok}, nil
	}
	if _, ok := hostbin.Look("cloudflared"); ok {
		return &shareTool{mode: shareModeCloudflare}, nil
	}
	if _, ok := hostbin.Look("expose"); ok {
		return &shareTool{mode: shareModeExpose}, nil
	}
	// Nothing is installed. A token is a deliberate choice of ngrok, so the
	// image stands in ahead of the signup-free SSH tools, but never ahead of a
	// tool already on the machine: pulling one is the slower way to the same URL.
	if ngrokToken != "" {
		return &shareTool{mode: shareModeNgrok, ngrok: ngrokRunner{container: true}, ngrokToken: ngrokToken}, nil
	}
	if _, ok := hostbin.Look("ssh"); ok {
		fmt.Println("ngrok/cloudflared/Expose not found — using localhost.run (SSH, no signup required)")
		return &shareTool{mode: shareModeSSH, ssh: sshProviders["localhost-run"]}, nil
	}

	return nil, fmt.Errorf("no tunnel tool found — install ngrok (https://ngrok.com/download), cloudflared (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/), or Expose (https://expose.dev), or ensure ssh is in PATH")
}

// defaultToolHint explains that a missing binary was picked by the configured
// default rather than by a flag, so a bare "lerd share" does not look broken.
func defaultToolHint(fromDefault bool) string {
	if !fromDefault {
		return ""
	}
	return "\nIt is your \"lerd share:tool\" default; run \"lerd share:tool auto\" to go back to auto-detection"
}

// cloudflaredCertPath returns the origin certificate cloudflared writes after
// "tunnel login". TUNNEL_ORIGIN_CERT overrides it, mirroring cloudflared itself.
func cloudflaredCertPath() string {
	if p := os.Getenv("TUNNEL_ORIGIN_CERT"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cloudflared", "cert.pem")
}

// ensureCloudflareTunnel makes sure a named tunnel exists for the site and that
// domain routes to it, logging in interactively first if needed. interactive is
// false for the dashboard, which has no terminal to run the login in and says so
// rather than hanging on a prompt nobody can see.
// Returns the tunnel name to pass to "cloudflared tunnel run".
func ensureCloudflareTunnel(siteName, domain string, interactive bool) (string, error) {
	if _, err := os.Stat(cloudflaredCertPath()); err != nil {
		if !interactive {
			return "", fmt.Errorf("cloudflared is not authorized for your Cloudflare account yet: run \"cloudflared tunnel login\" in a terminal once, then share again")
		}
		fmt.Println("cloudflared is not logged in yet, a browser window will open to authorize it.")
		login := exec.Command(hostbin.Path("cloudflared"), "tunnel", "login")
		login.Stdin = os.Stdin
		login.Stdout = os.Stdout
		login.Stderr = os.Stderr
		if err := login.Run(); err != nil {
			return "", fmt.Errorf("cloudflared tunnel login: %w", err)
		}
		if _, err := os.Stat(cloudflaredCertPath()); err != nil {
			return "", fmt.Errorf("cloudflared login finished but %s is still missing", cloudflaredCertPath())
		}
	}

	name := "lerd-" + siteName
	out, err := exec.Command(hostbin.Path("cloudflared"), "tunnel", "create", name).CombinedOutput()
	reused := err != nil && strings.Contains(string(out), "already exists")
	if err != nil && !reused {
		return "", fmt.Errorf("cloudflared tunnel create %s: %w\n%s", name, err, out)
	}
	if reused {
		if err := checkTunnelCredentials(name); err != nil {
			return "", err
		}
	}

	// Re-routing a hostname that already points here is a no-op, but cloudflared
	// refuses to overwrite a record aimed elsewhere, so tolerate that and let the
	// user verify where it points.
	out, err = exec.Command(hostbin.Path("cloudflared"), "tunnel", "route", "dns", name, domain).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "already exists") || strings.Contains(string(out), "already configured") {
			if interactive {
				fmt.Printf("DNS record for %s already exists, make sure it CNAMEs to tunnel %q.\n", domain, name)
			}
		} else {
			return "", fmt.Errorf("cloudflared tunnel route dns %s %s: %w\n%s", name, domain, err, out)
		}
	} else if interactive && strings.Contains(string(out), "Added CNAME") {
		fmt.Printf("Routed %s to tunnel %q. The record is new, so give it a moment: opening it too early can leave your resolver caching the miss for up to 30 minutes.\n", domain, name)
	}
	return name, nil
}

// checkTunnelCredentials verifies the credentials file for an existing tunnel is
// present locally. Without it "tunnel run" dies with an opaque cloudflared error,
// which is what happens when the tunnel was created on another machine.
func checkTunnelCredentials(name string) error {
	out, err := exec.Command(hostbin.Path("cloudflared"), "tunnel", "list", "--name", name, "--output", "json").Output()
	if err != nil {
		return nil // cannot tell, let "tunnel run" be the judge
	}
	var tunnels []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &tunnels); err != nil || len(tunnels) == 0 {
		return nil
	}
	cert := cloudflaredCertPath()
	if cert == "" {
		return nil
	}
	creds := filepath.Join(filepath.Dir(cert), tunnels[0].ID+".json")
	if _, err := os.Stat(creds); err == nil {
		return nil
	}
	return fmt.Errorf("tunnel %q exists on your Cloudflare account but its credentials file is missing at %s: copy it from the machine that created the tunnel, or run \"cloudflared tunnel delete %s\" and share again", name, creds, name)
}

// startHostProxy starts a local HTTP reverse proxy on a random loopback port.
// It rewrites the Host header to domain before forwarding to nginx,
// so nginx can route the request to the correct vhost.
// For secured (TLS) sites it connects to the HTTPS port to avoid the HTTP→HTTPS
// redirect loop; for plain HTTP sites it connects to httpPort.
// It also rewrites Location response headers, replacing the local domain with the
// tunnel host so browser redirects don't escape to the local .test URL.
// Returns the chosen port and a stop function.
func startHostProxy(domain string, httpPort, httpsPort int, secured bool) (int, func(), error) {
	var target *url.URL
	if secured {
		target = &url.URL{Scheme: "https", Host: fmt.Sprintf("localhost:%d", httpsPort)}
	} else {
		target = &url.URL{Scheme: "http", Host: fmt.Sprintf("localhost:%d", httpPort)}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// For secured sites the upstream uses a local mkcert cert — clone the default
	// transport (preserving connection pooling/timeouts) and add TLS skip-verify.
	if secured {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, ServerName: domain} //nolint:gosec
		proxy.Transport = t
	}

	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Save the incoming tunnel host before overwriting.
		tunnelHost := req.Host
		orig(req)
		// Tell Laravel (and any other framework) the real public host/scheme.
		req.Header.Set("X-Forwarded-Host", tunnelHost)
		req.Header.Set("X-Forwarded-Proto", "https")
		// Rewrite Host so nginx routes to the right vhost.
		req.Host = domain
		// Ask upstream to send uncompressed bodies so ModifyResponse can rewrite
		// asset URLs without having to decompress every text response.
		req.Header.Set("Accept-Encoding", "identity")
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		tunnelHost := resp.Request.Header.Get("X-Forwarded-Host")
		if tunnelHost == "" {
			return nil
		}

		// Rewrite the headers that carry an absolute URL back to the client.
		// An external redirect is not always a Location: a framework can hand
		// one to its own client through a header of its own instead.
		for _, h := range []string{"Location", "Content-Location", "X-Inertia-Location"} {
			if v := resp.Header.Get(h); v != "" {
				resp.Header.Set(h, rewriteTunnelURL(v, domain, tunnelHost))
			}
		}

		ct := resp.Header.Get("Content-Type")
		if !isTextContent(ct) {
			return nil
		}

		// Rewrite body for text content (HTML/CSS/JS) so absolute asset URLs
		// pointing to the local .test domain are replaced with the tunnel host.
		// Most upstreams honour Accept-Encoding: identity, but some (PHP-FPM
		// behind nginx with gzip on) compress anyway, so decode gzip ourselves.
		enc := resp.Header.Get("Content-Encoding")
		var body []byte
		var err error
		switch enc {
		case "", "identity":
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return err
			}
		case "gzip":
			gr, gErr := gzip.NewReader(resp.Body)
			if gErr != nil {
				return nil // leave untouched if we can't decompress
			}
			body, err = io.ReadAll(gr)
			gr.Close()
			resp.Body.Close()
			if err != nil {
				return err
			}
			resp.Header.Del("Content-Encoding")
		default:
			return nil // unknown encoding, leave untouched
		}

		body = rewriteTunnelBody(body, domain, tunnelHost)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}

	srv := &http.Server{Handler: proxy}
	go srv.Serve(ln) //nolint:errcheck

	port := ln.Addr().(*net.TCPAddr).Port
	return port, func() { srv.Close() }, nil
}

// rewriteTunnelURL swaps the site's own domain for the public one in a single
// URL. The tunnel is always TLS, so a plain-http local URL is upgraded rather
// than handed back as the mixed content a browser refuses to load.
func rewriteTunnelURL(v, domain, tunnelHost string) string {
	v = strings.ReplaceAll(v, "://"+domain, "://"+tunnelHost)
	if strings.HasPrefix(v, "http://"+tunnelHost) {
		v = "https://" + v[len("http://"):]
	}
	return v
}

// rewriteTunnelBody does the same across a whole response body. JSON escapes its
// slashes, so the same URL also reaches the browser as https:\/\/domain\/x, and
// a payload embedded in a page or returned from an XHR only carries that form.
func rewriteTunnelBody(body []byte, domain, tunnelHost string) []byte {
	for _, sep := range []string{"://", `:\/\/`} {
		for _, scheme := range []string{"https", "http"} {
			body = bytes.ReplaceAll(body,
				[]byte(scheme+sep+domain), []byte("https"+sep+tunnelHost))
		}
	}
	return body
}

// isTextContent returns true for Content-Type values whose body should be
// scanned and rewritten (HTML, CSS, JS, JSON).
func isTextContent(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "application/javascript") ||
		strings.Contains(ct, "text/javascript") ||
		strings.Contains(ct, "application/json")
}
