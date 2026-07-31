package ui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	lerdcli "github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"golang.org/x/crypto/bcrypt"
)

type ctxKeyRemoteDashboard struct{}

// loopbackOnlyRoutes are dashboard endpoints that perform actions too
// destructive or sensitive to hand to a remote (LAN) client on the strength
// of a password alone: shutting lerd down entirely, opening a terminal on
// the host, linking arbitrary host filesystem paths as new sites. The local
// user can still use them as normal, and a remote session reaches them only
// after `lerd remote-control full-access on`.
var loopbackOnlyRoutes = []string{
	"/api/lerd/stop",            // shuts down all lerd containers
	"/api/lerd/quit",            // exits the dashboard process
	"/api/lerd/update-terminal", // spawns a terminal emulator on the host
	"/api/logs/terminal",        // spawns a terminal emulator on the host
	"/api/sites/link",           // links arbitrary host filesystem paths
	"/api/browse",               // browses host filesystem
	"/api/push/test",            // fires notifications onto subscribed devices
}

// loopbackOnlyRoutePrefixes are endpoint subtrees restricted in full, so a
// new subresource cannot escape by failing to be listed. Databases read out,
// drop and overwrite the data the "/env" gate already protects.
var loopbackOnlyRoutePrefixes = []string{
	"/api/databases",
	"/api/entities",
	// Replaces executables on the host's PATH, so it stays with the terminal
	// and link routes rather than behind Basic auth alone.
	"/api/tools",
}

// loopbackOnlySiteSubactions are the per-site actions (under
// /api/sites/{domain}/) whose entire subtree is restricted. A subaction
// "/env" gates /api/sites/{d}/env and every nested route under it (e.g.
// /env/files, /env/backups, /env/backups/<name>, /env/restore), so adding a
// new subresource cannot accidentally escape the gate by failing to be
// re-listed here.
var loopbackOnlySiteSubactions = []string{
	"/terminal", // opens an interactive shell on the host
	"/env",      // raw .env content + backups + restore (APP_KEY, DB creds, tokens)
}

// isLoopbackOnlyPath reports whether the given URL path is in either the
// exact-match list or matches a per-site action whose entire subtree is
// restricted.
func isLoopbackOnlyPath(path string) bool {
	for _, p := range loopbackOnlyRoutes {
		if path == p {
			return true
		}
	}
	for _, p := range loopbackOnlyRoutePrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	if !strings.HasPrefix(path, "/api/sites/") {
		return false
	}
	rest := strings.TrimPrefix(path, "/api/sites/")
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return false
	}
	after := rest[slash:]
	for _, action := range loopbackOnlySiteSubactions {
		if after == action || strings.HasPrefix(after, action+"/") {
			return true
		}
	}
	return false
}

// remoteFullAccessEnabled reports whether authenticated remote sessions have
// been opted into host actions.
func remoteFullAccessEnabled() bool {
	cfg, _ := config.LoadGlobal()
	return cfg != nil && cfg.UI.RemoteFullAccess
}

// fromHost reports whether r's source IP belongs to one of the host's
// own interfaces. The mailpit container reaches the dashboard via
// host.containers.internal, which pasta (Linux) and gvproxy / vmnet
// (macOS) source-NAT to the host, so lerd-ui sees the request as coming
// from one of its own addresses. A LAN attacker arrives from a different
// IP and is rejected. Spoofing a host-owned address would break the TCP
// handshake because the SYN-ACK routes back into the host rather than
// reaching the attacker.
//
// Interfaces are re-read on every call so VPN attach, WiFi switch, or
// a late-arriving podman bridge are picked up without a daemon restart.
// Each call is a few syscalls, fine for webhook-rate traffic.
func fromHost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	// IPv6 link-local sources may carry a zone suffix (fe80::1%eth0);
	// strip it before parsing so the value compare below works.
	if i := strings.Index(host, "%"); i != -1 {
		host = host[:i]
	}
	src := net.ParseIP(host)
	if src == nil {
		return false
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP == nil {
			continue
		}
		if src.Equal(ipNet.IP) {
			return true
		}
	}
	return false
}

// unsafeMethod reports whether m can mutate server state and therefore must
// pass the cross-origin gate. Read-only methods (GET, HEAD, OPTIONS) can't,
// so a forged one does no harm.
func unsafeMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// csrfHeader is the request header lerd's own clients set to clear the
// cross-origin gate. Its presence is the proof; the value is ignored. The
// gate reads it here and withCORS advertises it in Access-Control-Allow-Headers
// so the split-origin dashboard's preflight succeeds.
const csrfHeader = "X-Lerd-CSRF"

// csrfExemptPath reports whether path skips the cross-origin gate. These
// endpoints are reached by non-browser clients (or cross-origin pages we
// can't control) that can't carry the header, and each already has its own
// source protection: /api/remote-setup has a token + RFC1918 + lockout gate,
// the mailpit webhook is restricted to host-NAT'd source IPs, the internal
// notify bridge (POSTed over loopback by out-of-process CLI/MCP commands) has
// its own loopback gate and only triggers a dashboard refresh, and the
// per-site unpause is POSTed from the paused-site holding page on the site's
// own domain and is non-destructive.
func csrfExemptPath(path string) bool {
	switch path {
	case "/api/remote-setup", "/api/webhooks/mailpit", "/api/internal/notify":
		return true
	}
	return strings.HasPrefix(path, "/api/sites/") && strings.HasSuffix(path, "/unpause")
}

// passesCSRF reports whether an unsafe-method request carries proof it was
// initiated by lerd's own dashboard or a trusted local client.
//
// Browsers attach Sec-Fetch-Site automatically and scripts cannot forge it:
// same-origin / same-site / none are first-party and pass; cross-site only
// passes when the Origin is one of lerd's own dashboard origins, because the
// lerd.localhost-to-localhost:7073 apiBase rewrite is itself labelled
// cross-site. A real attacker's Origin is never in the allowlist.
//
// Older browsers and non-browser callers omit Sec-Fetch; for those we require
// the X-Lerd-CSRF header. A cross-origin CORS-simple request (the RCE vector)
// can't set a custom header without a preflight, and lerd only answers
// preflight for its own origins, so the header's presence is proof enough.
func passesCSRF(r *http.Request) bool {
	if v, _ := r.Context().Value(ctxKeyUnixSocket{}).(bool); v {
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "same-site", "none":
		return true
	case "cross-site":
		return allowedCORSOrigins[r.Header.Get("Origin")]
	}
	return r.Header.Get(csrfHeader) != ""
}

// withRemoteControlGate wraps the dashboard mux with the LAN-access gate.
// Two independent flags control LAN access:
//
//   - cfg.LAN.Exposed   — "may LAN clients reach lerd at all?" (lerd lan:expose)
//   - cfg.UI.PasswordHash — "if they may, what credentials do they need?"
//     (lerd remote-control on)
//
// Behavior matrix for non-loopback requests:
//
//	cfg.LAN.Exposed | cfg.UI.PasswordHash | result
//	----------------|---------------------|--------------------------------
//	false           | empty               | 403 (LAN exposure off)
//	false           | set                 | 403 (LAN exposure off — credentials are inert)
//	true            | empty               | 403 (no credentials configured)
//	true            | set                 | require HTTP Basic auth
//
// Direct local dashboard requests bypass both checks. OPTIONS preflight
// passes through because it has no Authorization header. /api/remote-setup
// has its own token and IP gate.
//
// Beyond authentication, the loopbackOnlyRoutes list stays closed to remote
// clients unless cfg.UI.RemoteFullAccess is set. The local user keeps those
// routes either way.
func withRemoteControlGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. CORS preflight: pass through. Browsers don't include the
		// Authorization header on preflight, so requiring auth here would
		// break every cross-origin request from a configured client.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// 1b. Cross-origin (CSRF) gate. A state-changing request must prove it
		// came from lerd's own dashboard rather than a malicious page open in
		// the developer's browser. This is the one check that also applies to
		// loopback, because the RCE vector is exactly a local browser POSTing
		// to 127.0.0.1:7073/api/sites/<d>/tinker. Exempt endpoints are reached
		// by non-browser clients that have their own source protection.
		if unsafeMethod(r.Method) && !csrfExemptPath(r.URL.Path) && !passesCSRF(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Forbidden — cross-origin request blocked. Use the Lerd dashboard itself.", http.StatusForbidden)
			return
		}

		// 2. The remote-setup bootstrap endpoint has its own gate (token,
		// RFC 1918 source IP, brute-force lockout). It must remain reachable
		// from a remote laptop *before* the user has set up dashboard auth.
		if r.URL.Path == "/api/remote-setup" {
			next.ServeHTTP(w, r)
			return
		}

		// 2b. Mailpit's webhook is POSTed from inside the mailpit container
		// to host.containers.internal:7073. pasta (Linux) and gvproxy /
		// vmnet (macOS) source-NAT that to one of the host's own interface
		// IPs, so we accept any caller whose source IP belongs to the
		// host. A LAN attacker arrives from a different IP and is rejected,
		// closing the "anyone on the WiFi can spam fake mail pushes" vector.
		if r.URL.Path == "/api/webhooks/mailpit" && fromHost(r) {
			next.ServeHTTP(w, r)
			return
		}

		// 3. Only direct host control bypasses authentication. A reverse proxy
		// may connect from 127.0.0.1 on behalf of a remote browser.
		if isLocalControlRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		// 4. Non-loopback path. Inspect the configured LAN/remote-control
		// state. All gate responses set Cache-Control: no-store so
		// browsers don't replay an old 403/401 after the user enables
		// remote control or LAN exposure.
		cfg, _ := config.LoadGlobal()

		// 4pre. Host-action routes stay closed to remote clients unless the
		// user has explicitly opted in. This is checked before credentials
		// so an unopted install answers the same way whatever is guessed.
		if isLoopbackOnlyPath(r.URL.Path) && (cfg == nil || !cfg.UI.RemoteFullAccess) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Forbidden — this action is only available from the lerd host. Run `lerd remote-control full-access on` to allow it remotely.", http.StatusForbidden)
			return
		}

		// 4a. LAN exposure is the top-level gate. If lan:expose is off,
		// LAN clients are denied regardless of whether credentials are
		// set — this prevents stale credentials from a previous expose
		// session from surviving lan:unexpose, and matches the safe
		// default state of "lerd is invisible to the network".
		if cfg == nil || !cfg.LAN.Exposed {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Forbidden — lerd is not exposed to the LAN. Run `lerd lan:expose` on the server to enable LAN access.", http.StatusForbidden)
			return
		}

		// 4b. LAN exposure is on, but no remote-control credentials have
		// been configured. The dashboard is reachable but unauthenticated
		// access would be a free-for-all, so deny.
		if cfg.UI.PasswordHash == "" {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Forbidden — dashboard credentials are not configured. Run `lerd remote-control on` on the server to enable.", http.StatusForbidden)
			return
		}

		// 5. A valid session cookie authenticates without re-challenging.
		// It is issued after a Basic-auth success below and HMAC'd with the
		// password hash, so changing or clearing credentials invalidates it.
		// This is what stops iOS Safari, which drops cached Basic
		// credentials between refreshes, from prompting on every load.
		now := time.Now()
		if c, err := r.Cookie(remoteSessionCookie); err == nil &&
			remoteSessionValid(c.Value, cfg.UI.Username, cfg.UI.PasswordHash, now) {
			serveRemoteDashboard(next, w, r)
			return
		}

		// 6. Validate HTTP Basic auth.
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="lerd dashboard"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if subtle.ConstantTimeCompare([]byte(user), []byte(cfg.UI.Username)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="lerd dashboard"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(cfg.UI.PasswordHash), []byte(pass)) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="lerd dashboard"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Basic auth cleared — mint a session cookie so the browser skips
		// the challenge on subsequent requests.
		setRemoteSessionCookie(w, cfg.UI.Username, cfg.UI.PasswordHash, now)
		serveRemoteDashboard(next, w, r)
	})
}

func serveRemoteDashboard(next http.Handler, w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), ctxKeyRemoteDashboard{}, true)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// handleAccessMode serves /api/access-mode. It reports whether this request
// has dashboard-control authority and whether LAN exposure is enabled.
func handleAccessMode(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.LoadGlobal()
	lanExposed := cfg != nil && cfg.LAN.Exposed
	writeJSON(w, map[string]any{
		"local_control": hasDashboardControl(r),
		"lan_exposed":   lanExposed,
	})
}

// handleLANStatus serves /api/lan/status.
//
//	GET                               → { exposed, services_enabled, services_reachable, lan_ip }
//	POST { action: "expose" }         → exposes sites, DNS, and dashboard bind
//	POST { action: "unexpose" }       → returns every endpoint to loopback
//	POST { action: "services_on" }    → opts managed services into LAN access
//	POST { action: "services_off" }   → returns managed services to loopback
//
// POST requires dashboard-control authority because it rewrites runtime units
// and host configuration.
func handleLANStatus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := config.LoadGlobal()
		exposed := false
		servicesEnabled := false
		if cfg != nil {
			exposed = cfg.LAN.Exposed
			servicesEnabled = cfg.LAN.ServicesExposed
		}
		lanIP := ""
		if exposed {
			lanIP = uiPrimaryLANIP()
		}
		writeJSON(w, map[string]any{
			"exposed":            exposed,
			"services_enabled":   servicesEnabled,
			"services_reachable": exposed && servicesEnabled,
			"lan_ip":             lanIP,
			"macos":              runtime.GOOS == "darwin",
		})
		return

	case http.MethodPost:
		if !hasDashboardControl(r) {
			http.Error(w, "Forbidden — dashboard authentication is required to change LAN exposure.", http.StatusForbidden)
			return
		}
		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch body.Action {
		case "expose", "unexpose", "services_on", "services_off":
		default:
			http.Error(w, "unknown action — expected 'expose', 'unexpose', 'services_on', or 'services_off'", http.StatusBadRequest)
			return
		}

		// Stream NDJSON progress so the dashboard can render per-step
		// feedback instead of a single opaque spinner. Each line is a
		// JSON object: {step, status} for in-flight steps and a final
		// {result, exposed, lan_ip, error} envelope when the toggle
		// completes (or errors).
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeLine := func(payload map[string]any) {
			data, _ := json.Marshal(payload)
			_, _ = w.Write(append(data, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
		progress := func(step string) {
			writeLine(map[string]any{"step": step})
		}
		serviceState := func() (enabled, reachable bool) {
			cfg, _ := config.LoadGlobal()
			if cfg == nil {
				return false, false
			}
			return cfg.LAN.ServicesExposed, cfg.LAN.Exposed && cfg.LAN.ServicesExposed
		}

		switch body.Action {
		case "expose":
			lanIP, err := lerdcli.EnableLANExposure(progress)
			if err != nil {
				writeLine(map[string]any{"result": "error", "error": err.Error()})
				return
			}
			enabled, reachable := serviceState()
			writeLine(map[string]any{
				"result":             "ok",
				"exposed":            true,
				"services_enabled":   enabled,
				"services_reachable": reachable,
				"lan_ip":             lanIP,
			})
			return
		case "unexpose":
			if err := lerdcli.DisableLANExposure(progress); err != nil {
				writeLine(map[string]any{"result": "error", "error": err.Error()})
				return
			}
			enabled, _ := serviceState()
			writeLine(map[string]any{
				"result":             "ok",
				"exposed":            false,
				"services_enabled":   enabled,
				"services_reachable": false,
				"lan_ip":             "",
			})
			return
		case "services_on", "services_off":
			enabled := body.Action == "services_on"
			if err := lerdcli.SetManagedServiceLANExposure(enabled, progress); err != nil {
				writeLine(map[string]any{"result": "error", "error": err.Error()})
				return
			}
			serviceEnabled, reachable := serviceState()
			writeLine(map[string]any{
				"result":             "ok",
				"services_enabled":   serviceEnabled,
				"services_reachable": reachable,
			})
			return
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// uiPrimaryLANIP duplicates the dial-trick from cli/dns.go because importing
// cli from ui would risk a cycle. Cheap.
func uiPrimaryLANIP() string {
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP.String()
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if v4 := ipnet.IP.To4(); v4 != nil && !v4.IsLoopback() {
					return v4.String()
				}
			}
		}
	}
	return ""
}

// handleRemoteControl serves /api/remote-control. The middleware already
// gates this endpoint to loopback (because writing the password from a
// browser over HTTP would otherwise expose it to the network), so we don't
// need a second source-IP check here.
//
//	GET                                  → { enabled, username }
//	POST { action: "enable", username, password } → enables, persists hash
//	POST { action: "disable" }           → clears credentials
func handleRemoteControl(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.LoadGlobal()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"enabled":     cfg.UI.PasswordHash != "",
			"username":    cfg.UI.Username,
			"full_access": cfg.UI.RemoteFullAccess,
		})
		return

	case http.MethodPost:
		var body struct {
			Action   string `json:"action"`
			Username string `json:"username"`
			Password string `json:"password"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		cfg, err := config.LoadGlobal()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		switch body.Action {
		case "enable":
			// In disabled-DNS mode the dashboard chains "set credentials"
			// with "flip lan:expose" into a single user action because the
			// dashboard is effectively the only thing LAN exposure unlocks
			// (sites can't resolve over .localhost on remote devices). So we
			// only require lan:expose to be on first when DNS is enabled.
			if !cfg.LAN.Exposed && cfg.DNS.Enabled {
				http.Error(w, "LAN exposure is off — run `lerd lan:expose` first. Dashboard credentials are only meaningful while the dashboard is reachable from other devices.", http.StatusBadRequest)
				return
			}
			if body.Username == "" || body.Password == "" {
				http.Error(w, "username and password are required", http.StatusBadRequest)
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, "hashing password: "+err.Error(), http.StatusInternalServerError)
				return
			}
			cfg.UI.Username = body.Username
			cfg.UI.PasswordHash = string(hash)
			if err := config.SaveGlobal(cfg); err != nil {
				http.Error(w, "saving config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"ok": true, "enabled": true, "username": body.Username})
			return

		case "disable":
			cfg.UI.Username = ""
			cfg.UI.PasswordHash = ""
			cfg.UI.RemoteFullAccess = false
			if err := config.SaveGlobal(cfg); err != nil {
				http.Error(w, "saving config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"ok": true, "enabled": false, "full_access": false})
			return

		case "full-access":
			// Only the local dashboard may widen remote authority, so a
			// remote session can never grant itself host actions.
			if !isLocalControlRequest(r) {
				http.Error(w, "Forbidden — remote full access can only be changed from the lerd host.", http.StatusForbidden)
				return
			}
			if body.Enabled && cfg.UI.PasswordHash == "" {
				http.Error(w, "dashboard credentials are not configured — run `lerd remote-control on` first", http.StatusBadRequest)
				return
			}
			cfg.UI.RemoteFullAccess = body.Enabled
			if err := config.SaveGlobal(cfg); err != nil {
				http.Error(w, "saving config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"ok": true, "full_access": body.Enabled})
			return

		default:
			http.Error(w, "unknown action — expected 'enable', 'disable' or 'full-access'", http.StatusBadRequest)
			return
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// handleRemoteSetupGenerate serves /api/remote-setup/generate. POST creates a
// fresh one-time setup token for a remote machine. It requires dashboard-control
// authority. The corresponding /api/remote-setup endpoint consumed by that
// machine has its own RFC 1918 source and one-time-token gates.
func handleRemoteSetupGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !hasHostActionAuthority(r) {
		http.Error(w, "Forbidden — dashboard authentication is required to generate setup codes.", http.StatusForbidden)
		return
	}
	if cfg, _ := config.LoadGlobal(); cfg != nil && !cfg.DNS.Enabled {
		http.Error(w, "remote-setup requires lerd-managed DNS, the remote machine has no way to resolve *.localhost; set dns.enabled: true and re-run lerd install.", http.StatusBadRequest)
		return
	}

	code, err := lerdcli.GenerateRemoteSetupToken(15 * time.Minute)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lanIP := uiPrimaryLANIP()
	target := lanIP
	if target == "" {
		target = "<server-ip>"
	}
	curl := "curl -sSL 'http://" + target + ":7073/api/remote-setup?code=" + code + "' | bash"
	writeJSON(w, map[string]any{
		"code":       code,
		"lan_ip":     lanIP,
		"curl":       curl,
		"expires_in": "15m",
	})
}

// proxyHeaders are the headers a reverse proxy adds when it forwards a request
// on someone else's behalf. Their presence is what distinguishes a proxied
// request from a browser on the machine itself, since both arrive from 127.0.0.1.
var proxyHeaders = []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip", "Forwarded"}

// forwardedByProxy reports whether r carries evidence of having been relayed.
func forwardedByProxy(r *http.Request) bool {
	for _, h := range proxyHeaders {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// isLocalControlRequest reports whether a request may control the lerd host.
// Unix-socket requests and requests carrying the private nginx trust token are
// authoritative. A direct TCP request qualifies when its peer is loopback and
// it carries no forwarding headers, which rejects reverse proxies such as
// Tailscale Serve that connect from 127.0.0.1 for a remote browser.
//
// The Host header is deliberately not part of this: a local browser may reach
// the dashboard by the machine's own hostname (Debian and Ubuntu map it to
// 127.0.1.1) or any /etc/hosts alias, and locking those out would leave the
// local user with no way in.

func hasValidTrustToken(r *http.Request) bool {
	claimed := r.Header.Get("X-Lerd-Trust")
	if claimed == "" {
		return false
	}
	token, err := nginx.LoadOrGenerateTrustToken()
	return err == nil && token != "" &&
		subtle.ConstantTimeCompare([]byte(claimed), []byte(token)) == 1
}

func isLocalControlRequest(r *http.Request) bool {
	if v, _ := r.Context().Value(ctxKeyUnixSocket{}).(bool); v {
		return true
	}
	if hasValidTrustToken(r) {
		return true
	}
	if forwardedByProxy(r) {
		return false
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	ip := net.ParseIP(peer)
	return ip != nil && ip.IsLoopback()
}

// remoteSessionMayActOnHost reports whether r is an authenticated remote
// dashboard session that the user has opted into host actions.
func remoteSessionMayActOnHost(r *http.Request) bool {
	authenticated, _ := r.Context().Value(ctxKeyRemoteDashboard{}).(bool)
	return authenticated && remoteFullAccessEnabled()
}

// hasHostActionAuthority reports whether r may perform an action that reaches
// the host itself: executing commands, reading raw .env content, touching the
// filesystem, deleting captured data. The local dashboard always may; a remote
// session only after `lerd remote-control full-access on`. The middleware has
// already applied the stricter local check to anything that reaches a handler,
// so the loopback test here is the peer one.
func hasHostActionAuthority(r *http.Request) bool {
	return isLoopbackRequest(r) || remoteSessionMayActOnHost(r)
}

// hasDashboardControl is hasHostActionAuthority for the handlers that must
// hold up on their own, without the middleware in front: it rejects a reverse
// proxy connecting from 127.0.0.1 on behalf of a remote browser.
func hasDashboardControl(r *http.Request) bool {
	return isLocalControlRequest(r) || remoteSessionMayActOnHost(r)
}

// isLoopbackRequest reports whether r originates from the local host. Three
// paths qualify:
//
//  1. The connection arrived over the unix socket listener. Only host
//     processes with filesystem access to the socket can connect, so this is
//     at least as trusted as TCP loopback. The lerd.localhost nginx vhost
//     reaches lerd-ui via this path.
//  2. The TCP peer is a loopback IP (127.x, ::1). This catches direct visits
//     to http://localhost:7073 / http://127.0.0.1:7073.
//  3. The request carries an X-Lerd-Trust header whose value matches the
//     per-install token. Kept for backward compatibility with old vhosts
//     that may still inject the header; new installs use the unix socket.
func isLoopbackRequest(r *http.Request) bool {
	if v, _ := r.Context().Value(ctxKeyUnixSocket{}).(bool); v {
		return true
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if ip := net.ParseIP(peer); ip != nil && ip.IsLoopback() {
		return true
	}
	return hasValidTrustToken(r)
}
