package config

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

// presetFileGenerators maps a preset FileMount's `generator:` name to the Go
// function that renders it at materialise time. This is the one part of a file
// mount that can't be static YAML: dynamic contents like pgAdmin's family-
// discovered servers.json. External presets reference these by name; shipping a
// genuinely new generator still requires a lerd release, the deliberate boundary
// that keeps store presets from carrying executable discovery logic.
var presetFileGenerators = map[string]func(*CustomService) (string, error){
	"pgadmin_servers": pgadminServersJSON,
	"pgadmin_pgpass":  pgadminPgpass,
}

// DashboardProxyPrefix is the lerd-ui mount under which bundled admin
// dashboards are served same-origin so their cookies stay first-party in the
// iframe overlay. Shared by the lerd-ui proxy and the quadlet generator, which
// configures each upstream to serve its UI there.
const DashboardProxyPrefix = "/_svc/"

// DashboardProxyPath is the same-origin mount path for a proxied dashboard.
func DashboardProxyPath(name string) string {
	return DashboardProxyPrefix + name + "/"
}

// DashboardProxied reports whether svc's dashboard is served through lerd-ui's
// same-origin proxy rather than opened at its own origin. Only a bundled preset
// (Preset set and still resolvable) that asked for it qualifies; a user-defined
// custom service with dashboard_external keeps the new-tab behavior.
//
// Either flag asks for the proxy. They differ in what an older binary does with
// them, which is why a preset picks one deliberately: see Preset.DashboardProxy.
//
// The flag is read from the preset rather than the copy saved when the service
// was installed, so a store definition that moves a dashboard behind the proxy
// reaches services installed before it, the way file mounts do.
//
// Everything that puts a service behind the proxy has to agree with this, or
// the two halves land apart: telling an upstream to serve under the mount while
// the dashboard still opens at its own root leaves the app answering nowhere the
// UI looks.
func DashboardProxied(svc *CustomService) bool {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	p, err := LoadPreset(svc.Preset)
	return err == nil && (p.DashboardProxy || p.DashboardExternal || p.DashboardProxyStrip)
}

// DashboardProxyStrips reports whether this service's dashboard is served by
// stripping the mount prefix rather than forwarding it. Read from the preset,
// like DashboardProxied, so a store change reaches installs that never reinstall.
func DashboardProxyStrips(svc *CustomService) bool {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	p, err := LoadPreset(svc.Preset)
	return err == nil && p.DashboardProxyStrip
}

// DashboardProxyRebases returns the HTML attributes whose root-absolute values
// the proxy rewrites onto the mount for this service. Read from the preset, like
// the flags above.
func DashboardProxyRebases(svc *CustomService) []string {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return nil
	}
	p, err := LoadPreset(svc.Preset)
	if err != nil {
		return nil
	}
	return p.DashboardProxyRebase
}

// DashboardProxyReroutes reports whether the page's own requests are funnelled
// back into the mount. Read from the preset, like the flags above.
func DashboardProxyReroutes(svc *CustomService) bool {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	p, err := LoadPreset(svc.Preset)
	return err == nil && p.DashboardProxyReroute
}

// DashboardRerouteScript returns an inline <script> that sends the page's own
// root-absolute requests through the mount. It runs before the app's scripts, so
// a URL the app computes from its origin (Meilisearch's mini-dashboard asks
// window.location.origin for the API host) reaches the upstream rather than
// lerd's own root. Requests already inside the mount, other origins and relative
// URLs are left exactly as they are.
func DashboardRerouteScript(name string) string {
	mount := strings.TrimSuffix(DashboardProxyPath(name), "/")
	return "<script>(function(){var m=" + strconv.Quote(mount) + ";" +
		"function r(u){try{u=String(u);}catch(e){return u;}" +
		"var o=location.origin;" +
		"if(u.indexOf(o+'/')===0){u=u.slice(o.length);}" +
		"else if(u.charAt(0)!=='/'||u.charAt(1)==='/'){return u;}" +
		"if(u===m||u.indexOf(m+'/')===0){return u;}" +
		"return m+u;}" +
		"var f=window.fetch;window.fetch=function(i,o){try{" +
		"if(i&&typeof i==='object'&&i.url){i=new Request(r(i.url),i);}else{i=r(i);}" +
		"}catch(e){}return f.call(this,i,o);};" +
		"var x=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(){" +
		"try{arguments[1]=r(arguments[1]);}catch(e){}return x.apply(this,arguments);};" +
		"})();</script>"
}

// DefaultPresetService describes a default-stack service the way the proxy and
// the dashboard link expect a bundled preset: as the CustomService a preset
// install would have written. A default service has no such file, since lerd
// ships it rather than the user adding it, which is the only reason the proxy
// could not reach one. Returns nil for anything that is not a default preset.
func DefaultPresetService(name string) *CustomService {
	if !IsDefaultPreset(name) {
		return nil
	}
	dash := DefaultPresetDashboard(name)
	if dash == "" {
		return nil
	}
	return &CustomService{Name: name, Preset: name, Dashboard: dash, Ports: PresetPorts(name)}
}

// PresetProxyEnv returns the container env that makes a bundled upstream serve
// its UI under the same /_svc/<name> path the lerd-ui proxy mounts it at, so
// the dashboard embeds same-origin. It is injected at quadlet generation (not
// stored in the service YAML) so existing installs pick it up on the next
// start without a reinstall, mirroring how PresetFiles are re-sourced. Keeping
// it out of the YAML also matters for compatibility: both values move the app
// off "/", and a binary that predates the proxy still opens the dashboard
// there. Returns ok=false for presets that configure the prefix another way:
// rabbitmq uses a management.path_prefix conf mount and phpmyadmin an apache
// Alias (see presetFiles), pgadmin reads a per-request header (see
// PresetProxyHeader).
func PresetProxyEnv(svc *CustomService) (key, value string, ok bool) {
	if !DashboardProxied(svc) {
		return "", "", false
	}
	switch svc.Preset {
	case "redisinsight":
		return "RI_PROXY_PATH", strings.TrimSuffix(DashboardProxyPath(svc.Name), "/"), true
	case "mongo-express":
		// Its router is mounted at site.baseUrl, which the config expects with
		// both slashes.
		return "ME_CONFIG_SITE_BASEURL", DashboardProxyPath(svc.Name), true
	}
	return "", "", false
}

// PresetProxyHeader returns a request header the lerd-ui proxy must add so a
// bundled upstream serves its UI under the /_svc/<name> mount. It is the same
// job as PresetProxyEnv for apps that take the prefix per request rather than
// at boot, which is the better half of the deal: the prefix applies without
// restarting the container, and the app keeps answering at "/" alongside it.
// pgAdmin's ReverseProxied middleware reads X-Script-Name, strips it from the
// path and prefixes every URL it generates.
func PresetProxyHeader(svc *CustomService) (key, value string, ok bool) {
	if !DashboardProxied(svc) {
		return "", "", false
	}
	switch svc.Preset {
	case "pgadmin":
		return "X-Script-Name", strings.TrimSuffix(DashboardProxyPath(svc.Name), "/"), true
	}
	return "", "", false
}

// PresetDashboardBootstrap returns an inline <script> to inject into the
// proxied dashboard's HTML so it opens already authenticated, mirroring how
// pgadmin/phpmyadmin auto-log-in via config. Returns "" when the dashboard
// needs no client-side priming.
//
// RabbitMQ's management UI (3.13) keeps no server session: the login form just
// stores HTTP Basic credentials in localStorage plus a `loggedIn` marker packed
// into its `m` cookie under a runtime-hashed key. We seed the same state before
// its scripts run. The cookie key is derived with the app's own hashCode/
// short_key algorithm at runtime (replicated inline) so it stays correct across
// versions; the page's CSP already allows unsafe-inline scripts.
func PresetDashboardBootstrap(svc *CustomService) string {
	if svc == nil {
		return ""
	}
	switch svc.Preset {
	case "rabbitmq":
		user := svc.Environment["RABBITMQ_DEFAULT_USER"]
		if user == "" {
			user = "root"
		}
		pass := svc.Environment["RABBITMQ_DEFAULT_PASS"]
		if pass == "" {
			pass = "lerd"
		}
		creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		return "<script>(function(){try{" +
			"if(localStorage.getItem('rabbitmq.credentials'))return;" +
			"function hc(s){var h=0;for(var i=0;i<s.length;i++){h=(31*h+s.charCodeAt(i))|0;}return h;}" +
			"localStorage.setItem('rabbitmq.credentials','" + creds + "');" +
			"localStorage.setItem('rabbitmq.auth-scheme','Basic');" +
			"document.cookie='m='+(Math.abs((hc('loggedIn')<<16)>>16).toString(16))+':true; path=/';" +
			"}catch(e){}})();</script>"
	}
	return ""
}

// PresetFiles returns the file mounts declared in the named preset's YAML, with
// each mount's `generator:` resolved to its ContentFn. It reads the preset fresh
// (embed bundle or store cache) so updating lerd, or the store definition, rolls
// out new file contents on the next service start without a reinstall. A mount
// naming an unknown generator is skipped rather than mounted empty, so a store
// preset built for a newer lerd degrades gracefully. Only presets carry files;
// custom services have any files: block stripped on load (see LoadCustomService).
func PresetFiles(presetName string) []FileMount {
	p, err := LoadPreset(presetName)
	if err != nil || len(p.Files) == 0 {
		return nil
	}
	out := make([]FileMount, 0, len(p.Files))
	for _, f := range p.Files {
		if f.Generator != "" {
			gen, ok := presetFileGenerators[f.Generator]
			if !ok {
				continue
			}
			f.ContentFn = gen
		}
		out = append(out, f)
	}
	return out
}

// pgadminFriendlyName turns a container hostname like "lerd-postgres-18"
// into a human-friendly server label "Lerd Postgres 18".
func pgadminFriendlyName(host string) string {
	parts := strings.Split(strings.TrimPrefix(host, "lerd-"), "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return "Lerd " + strings.Join(parts, " ")
}

// pgadminPostgresHosts returns the postgres family members, falling back to
// the canonical lerd-postgres when discovery is empty (fresh install before
// the family registry has been populated).
func pgadminPostgresHosts() []string {
	hosts := ServicesInFamily("postgres")
	if len(hosts) == 0 {
		return []string{"lerd-postgres"}
	}
	return hosts
}

// pgadminServersJSON renders pgAdmin's servers.json with every installed
// postgres family member, so alternates like postgres-18 appear in the
// dashboard alongside the canonical postgres without manual server setup.
func pgadminServersJSON(_ *CustomService) (string, error) {
	type server struct {
		Name          string `json:"Name"`
		Group         string `json:"Group"`
		Host          string `json:"Host"`
		Port          int    `json:"Port"`
		MaintenanceDB string `json:"MaintenanceDB"`
		Username      string `json:"Username"`
		SSLMode       string `json:"SSLMode"`
		PassFile      string `json:"PassFile"`
	}
	servers := map[string]server{}
	for i, host := range pgadminPostgresHosts() {
		servers[strconv.Itoa(i+1)] = server{
			Name:          pgadminFriendlyName(host),
			Group:         "Servers",
			Host:          host,
			Port:          5432,
			MaintenanceDB: "postgres",
			Username:      "postgres",
			SSLMode:       "prefer",
			PassFile:      "/pgpass",
		}
	}
	data, err := json.MarshalIndent(map[string]any{"Servers": servers}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

// pgadminPgpass renders a libpq passfile with one line per postgres family
// member so pgAdmin's PassFile=/pgpass entry auto-logs every alternate.
func pgadminPgpass(_ *CustomService) (string, error) {
	var b strings.Builder
	for _, host := range pgadminPostgresHosts() {
		b.WriteString(host)
		b.WriteString(":5432:*:postgres:lerd\n")
	}
	return b.String(), nil
}
