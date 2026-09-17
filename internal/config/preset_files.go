package config

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"sort"
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

// DashboardProxyAtOwnPath reports whether this dashboard is served at the path
// its own build expects rather than at the /_svc/<name>/ mount.
func DashboardProxyAtOwnPath(svc *CustomService) bool {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	p, err := LoadPreset(svc.Preset)
	return err == nil && p.DashboardProxyAtPath
}

// DashboardProxyKeepsHost reports whether the browser's Host is forwarded as it
// arrived. A signed API needs it, the signature covering that header.
func DashboardProxyKeepsHost(svc *CustomService) bool {
	if svc == nil || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	p, err := LoadPreset(svc.Preset)
	return err == nil && p.DashboardProxyKeepHost
}

// DashboardMountPath is the path lerd-ui serves this dashboard at: the path the
// dashboard URL names when the preset asks for its own, and the /_svc/<name>/
// mount otherwise.
func DashboardMountPath(svc *CustomService) string {
	if !DashboardProxyAtOwnPath(svc) {
		return DashboardProxyPath(svc.Name)
	}
	u, err := url.Parse(svc.Dashboard)
	if err != nil || u.Path == "" || u.Path == "/" {
		return DashboardProxyPath(svc.Name)
	}
	if !strings.HasSuffix(u.Path, "/") {
		return u.Path + "/"
	}
	return u.Path
}

// DashboardMounts returns the paths lerd-ui serves dashboards at for the
// presets that ask to be served where their own build expects, keyed by service
// name. Both the proxy and the lerd.localhost vhost read it, so the path lerd
// answers at and the path nginx forwards cannot drift apart.
func DashboardMounts() map[string]string {
	mounts := map[string]string{}
	add := func(svc *CustomService) {
		if svc == nil || !DashboardProxied(svc) || !DashboardProxyAtOwnPath(svc) {
			return
		}
		mounts[svc.Name] = DashboardMountPath(svc)
	}
	for _, name := range DefaultPresetNames() {
		add(DefaultPresetService(name))
	}
	if custom, err := ListCustomServices(); err == nil {
		for _, svc := range custom {
			add(svc)
		}
	}
	return mounts
}

// DashboardLoginScript returns an inline <script> that fills the dashboard's own
// login form with the credentials lerd provisioned the service with and submits
// it. The form belongs to a framework that tracks its inputs in JavaScript, so a
// value is written through the native setter and announced, the way a keystroke
// would be, rather than assigned and left unnoticed.
func DashboardLoginScript(svc *CustomService) string {
	if svc == nil || svc.Preset == "" {
		return ""
	}
	p, err := LoadPreset(svc.Preset)
	if err != nil || p.DashboardLogin == nil {
		return ""
	}
	login := p.DashboardLogin
	pairs := make([][2]string, 0, len(login.Fields))
	for selector, envKey := range login.Fields {
		value := svc.Environment[envKey]
		if value == "" {
			value = presetEnvDefault(p, envKey)
		}
		if value == "" {
			return "" // A credential lerd does not hold; leave the form alone.
		}
		pairs = append(pairs, [2]string{selector, value})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	fields, err := json.Marshal(pairs)
	if err != nil {
		return ""
	}
	return "<script>(function(){var F=" + string(fields) +
		",P=" + strconv.Quote(login.Path) +
		",S=" + strconv.Quote(login.Submit) +
		",D=" + strconv.Quote(login.Done) + ",sent=false;" +
		"function go(){if(sent)return true;" +
		"try{if(D&&localStorage.getItem(D))return true;}catch(e){}" +
		"if(P&&location.pathname.indexOf(P)!==0)return false;" +
		"var b=document.querySelector(S);if(!b)return false;" +
		"var set=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value').set;" +
		"for(var i=0;i<F.length;i++){var el=document.querySelector(F[i][0]);if(!el)return false;" +
		"set.call(el,F[i][1]);el.dispatchEvent(new Event('input',{bubbles:true}));}" +
		"sent=true;b.click();return true;}" +
		"var n=0,t=setInterval(function(){if(go()||++n>100)clearInterval(t);},100);" +
		"document.addEventListener('DOMContentLoaded',go);})();</script>"
}

// presetEnvDefault reads a credential from the preset's own environment block,
// for a service installed before the value was ever written to its file.
func presetEnvDefault(p *Preset, key string) string {
	if p == nil {
		return ""
	}
	return p.Environment[key]
}

// DashboardRerouteScript returns an inline <script> that sends the page's own
// root-absolute requests through the mount. It runs before the app's scripts, so
// a URL the app computes from its origin (Meilisearch's mini-dashboard asks
// window.location.origin for the API host) reaches the upstream rather than
// lerd's own root. Requests already inside the mount, other origins and relative
// URLs are left exactly as they are.
func DashboardRerouteScript(name, keep string) string {
	mount := strings.TrimSuffix(DashboardProxyPath(name), "/")
	return "<script>(function(){var m=" + strconv.Quote(mount) + ",k=" + strconv.Quote(keep) + ";" +
		"function r(u){try{u=String(u);}catch(e){return u;}" +
		"var o=location.origin;" +
		"if(u.indexOf(o+'/')===0){u=u.slice(o.length);}" +
		"else if(u.charAt(0)!=='/'||u.charAt(1)==='/'){return u;}" +
		"if(u===m||u.indexOf(m+'/')===0){return u;}" +
		"if(k&&u.indexOf(k)===0){return u;}" +
		"return m+u;}" +
		// Rebuilding a Request from a Request turns its body into a stream, which
		// a browser will not send over HTTP/1.1, so the body is read first and
		// passed as it was.
		"var f=window.fetch;window.fetch=function(i,o){" +
		"try{" +
		"if(i&&typeof i==='object'&&i.url){var n=r(i.url);if(n===i.url){return f.call(this,i,o);}" +
		"var self=this;return i.arrayBuffer().then(function(b){" +
		"var init={method:i.method,headers:i.headers,mode:i.mode==='navigate'?'same-origin':i.mode," +
		"credentials:i.credentials,cache:i.cache,redirect:i.redirect,integrity:i.integrity};" +
		"if(i.method!=='GET'&&i.method!=='HEAD'){init.body=b;}" +
		"return f.call(self,n,init);});}" +
		"i=r(i);}catch(e){}return f.call(this,i,o);};" +
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
