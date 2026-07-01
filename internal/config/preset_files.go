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
// dashboards (rabbitmq, redisinsight) are served same-origin so their cookies
// stay first-party in the iframe overlay. Shared by the lerd-ui proxy and the
// quadlet generator, which configures each upstream to serve its UI there.
const DashboardProxyPrefix = "/_svc/"

// DashboardProxyPath is the same-origin mount path for a proxied dashboard.
func DashboardProxyPath(name string) string {
	return DashboardProxyPrefix + name + "/"
}

// PresetProxyEnv returns the container env that makes a bundled upstream serve
// its UI under the same /_svc/<name> path the lerd-ui proxy mounts it at, so
// the dashboard embeds same-origin. It is injected at quadlet generation (not
// stored in the service YAML) so existing installs pick it up on the next
// start without a reinstall, mirroring how PresetFiles are re-sourced. Returns
// ok=false for presets that configure the prefix another way: rabbitmq uses a
// management.path_prefix conf mount (see presetFiles).
func PresetProxyEnv(svc *CustomService) (key, value string, ok bool) {
	if svc == nil {
		return "", "", false
	}
	switch svc.Preset {
	case "redisinsight":
		return "RI_PROXY_PATH", strings.TrimSuffix(DashboardProxyPath(svc.Name), "/"), true
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
