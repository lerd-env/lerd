package config

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

// presetFiles holds the file mounts shipped with each bundled preset. This
// lives in Go rather than the preset YAMLs so that new lerd versions can
// update the mounted file contents automatically on the next service start
// without the user having to remove and reinstall the preset.
//
// Files are intentionally not a user feature: the three built-in presets
// below are the only ones that need runtime-generated config. A custom
// service author cannot declare their own file mounts.
var presetFiles = map[string][]FileMount{
	"rabbitmq": {
		{
			// lerd-ui proxies the management UI same-origin under /_svc/rabbitmq/
			// so its Cowboy session cookie stays first-party in the iframe. Mount
			// the UI at that same prefix so its asset/API links resolve there.
			Target:  "/etc/rabbitmq/conf.d/10-lerd-path-prefix.conf",
			Content: "management.path_prefix = /_svc/rabbitmq\n",
		},
	},
	"mysql": {
		{
			Target: "/etc/mysql/conf.d/lerd.cnf",
			// loose- prefix skips directives unknown to a given mysql version;
			// authentication_policy is omitted because mysql 9.x removed
			// mysql_native_password, which made the variable refuse to load.
			Content: `[mysqld]
character-set-server=utf8mb4
collation-server=utf8mb4_unicode_ci
innodb_file_per_table=ON
innodb_strict_mode=OFF
loose-innodb_default_row_format=DYNAMIC
loose-mysql-native-password=ON
loose-restrict-fk-on-non-standard-key=OFF
`,
		},
	},
	"pgadmin": {
		{
			Target:    "/pgadmin4/servers.json",
			ContentFn: pgadminServersJSON,
		},
		{
			Target:    "/pgpass",
			Mode:      "0600",
			Chown:     true,
			ContentFn: pgadminPgpass,
		},
		{
			Target: "/pgadmin4/config_local.py",
			Content: `X_FRAME_OPTIONS = ''
ENHANCED_COOKIE_PROTECTION = False
WTF_CSRF_CHECK_DEFAULT = False

# Allow pgadmin's Flask session + CSRF cookies to flow inside a cross-origin
# iframe (the lerd-ui dashboard). SameSite=None requires Secure=True, which
# browsers accept over HTTP on localhost.
SESSION_COOKIE_SAMESITE = 'None'
SESSION_COOKIE_SECURE = True
`,
		},
	},
	"phpmyadmin": {
		{
			Target: "/etc/phpmyadmin/config.user.inc.php",
			Content: `<?php
// Allow phpmyadmin's session cookie to be sent when it's embedded in
// an iframe served from a different origin (the lerd-ui dashboard).
// The default SameSite=Strict drops the cookie on form POSTs, which
// breaks the server-switch dropdown via CSRF token mismatch.
// SameSite=None requires Secure=1, which phpmyadmin only sets when
// isHttps() is true, so we force the HTTPS env var — browsers treat
// localhost as secure so Secure cookies are accepted over HTTP.
$cfg['CookieSameSite'] = 'None';
$_SERVER['HTTPS'] = 'on';

// The official phpmyadmin image only handles PMA_USER/PMA_PASSWORD for
// single-host setups; in multi-host (PMA_HOSTS) it writes host/verbose
// per server but leaves user/password blank, forcing a login screen.
// Rebuild $cfg['Servers'] from our own parallel env arrays so every
// discovered mysql/mariadb service auto-logs in with config auth.
$hosts = array_values(array_filter(array_map('trim', explode(',', (string) getenv('PMA_HOSTS')))));
$users = array_map('trim', explode(',', (string) getenv('PMA_USERS')));
$passwords = array_map('trim', explode(',', (string) getenv('PMA_PASSWORDS')));
foreach ($hosts as $i => $host) {
    $idx = $i + 1;
    $cfg['Servers'][$idx] = [
        'host'      => $host,
        'verbose'   => $host,
        'auth_type' => 'config',
        'user'      => $users[$i] ?? 'root',
        'password'  => $passwords[$i] ?? 'lerd',
        'AllowNoPassword' => false,
    ];
}
$cfg['AllowThirdPartyFraming'] = true;
`,
		},
	},
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

// PresetFiles returns the hardcoded file mounts for the named preset, or nil
// when the preset has no files. The returned slice is a copy so callers
// cannot mutate the shared definition.
func PresetFiles(presetName string) []FileMount {
	src := presetFiles[presetName]
	if len(src) == 0 {
		return nil
	}
	out := make([]FileMount, len(src))
	copy(out, src)
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
