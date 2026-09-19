package config

import (
	"strings"
	"testing"
)

// Phase 6 parity: an external store preset can ship its own static file mount
// purely in YAML, no Go change required.
func TestPresetFiles_ExternalStorePresetShipsFiles(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "ext-svc", "name: ext-svc\nimage: example/ext:1\nfiles:\n  - target: /etc/ext.conf\n    content: |\n      hello = world\n")
	files := PresetFiles("ext-svc")
	if len(files) != 1 || files[0].Target != "/etc/ext.conf" {
		t.Fatalf("external preset files = %+v", files)
	}
	if !strings.Contains(files[0].Content, "hello = world") {
		t.Errorf("content = %q, want it to contain the mounted body", files[0].Content)
	}
}

// A mount naming an unknown generator (e.g. a store preset built for a newer
// lerd) is skipped, never mounted empty.
func TestPresetFiles_UnknownGeneratorSkipped(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "gen-svc", "name: gen-svc\nimage: example/gen:1\nfiles:\n  - target: /a\n    content: static\n  - target: /b\n    generator: does-not-exist\n")
	files := PresetFiles("gen-svc")
	if len(files) != 1 || files[0].Target != "/a" {
		t.Errorf("unknown generator must be skipped, got %+v", files)
	}
}

// A known generator name resolves to its Go ContentFn.
func TestPresetFiles_KnownGeneratorResolves(t *testing.T) {
	found := false
	for _, f := range PresetFiles("pgadmin") {
		if f.Target == "/pgadmin4/servers.json" {
			found = true
			if f.ContentFn == nil {
				t.Error("pgadmin_servers generator did not resolve to a ContentFn")
			}
		}
	}
	if !found {
		t.Error("pgadmin servers.json mount missing")
	}
}

func TestPgadminServersJSON_listsEveryFamilyMember(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	// Built-in postgres + one alternate exercises the family-discovery path.
	if err := SaveCustomService(&CustomService{
		Name:   "postgres-18",
		Image:  "docker.io/postgis/postgis:18-3.6-alpine",
		Family: "postgres",
	}); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	out, err := pgadminServersJSON(nil)
	if err != nil {
		t.Fatalf("pgadminServersJSON: %v", err)
	}
	for _, want := range []string{
		`"Host": "lerd-postgres"`,
		`"Host": "lerd-postgres-18"`,
		`"Name": "Lerd Postgres"`,
		`"Name": "Lerd Postgres 18"`,
		`"Port": 5432`,
		`"PassFile": "/pgpass"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("servers.json missing %q\n%s", want, out)
		}
	}
}

func TestPgadminPgpass_oneLinePerFamilyMember(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := SaveCustomService(&CustomService{
		Name:   "postgres-17",
		Image:  "docker.io/postgis/postgis:17-3.6-alpine",
		Family: "postgres",
	}); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	out, err := pgadminPgpass(nil)
	if err != nil {
		t.Fatalf("pgadminPgpass: %v", err)
	}
	for _, want := range []string{
		"lerd-postgres:5432:*:postgres:lerd",
		"lerd-postgres-17:5432:*:postgres:lerd",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pgpass missing %q\n%s", want, out)
		}
	}
}

func TestPgadminPreset_consumesPostgresFamily(t *testing.T) {
	// dynamic_env wires pgadmin into the family-consumer regeneration path,
	// so installing/removing a postgres alternate triggers a servers.json
	// rebuild and a pgadmin restart.
	p, err := LoadPreset("pgadmin")
	if err != nil {
		t.Fatalf("LoadPreset(pgadmin): %v", err)
	}
	if got := p.DynamicEnv["LERD_POSTGRES_HOSTS"]; got != "discover_family:postgres" {
		t.Errorf("pgadmin must declare discover_family:postgres dynamic_env, got %q", got)
	}
	if p.Environment["PGADMIN_REPLACE_SERVERS_ON_STARTUP"] != "True" {
		t.Errorf("pgadmin must set PGADMIN_REPLACE_SERVERS_ON_STARTUP=True so the regenerated servers.json gets re-imported on restart")
	}
}

func TestRabbitMQPresetMountsPathPrefix(t *testing.T) {
	files := PresetFiles("rabbitmq")
	if len(files) == 0 {
		t.Fatal("rabbitmq preset has no file mounts")
	}
	f := files[0]
	if f.Target != "/etc/rabbitmq/conf.d/10-lerd-path-prefix.conf" {
		t.Errorf("rabbitmq conf mounted at %q, want /etc/rabbitmq/conf.d/10-lerd-path-prefix.conf", f.Target)
	}
	// The management UI must serve under the same prefix the lerd-ui proxy
	// mounts it at, or the iframe loads a blank shell (absolute asset paths).
	if !strings.Contains(f.Content, "management.path_prefix = /_svc/rabbitmq") {
		t.Errorf("rabbitmq conf missing management.path_prefix = /_svc/rabbitmq\n%s", f.Content)
	}
}

func TestRedisInsightProxyEnvInjectedByPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// RI_PROXY_PATH is injected at quadlet generation from the preset, not
	// stored in the service YAML, so existing installs serve under the proxy
	// mount after a restart without a reinstall.
	svc := &CustomService{Name: "redisinsight", Preset: "redisinsight", Dashboard: "http://localhost:8085", DashboardExternal: true}
	k, v, ok := PresetProxyEnv(svc)
	if !ok || k != "RI_PROXY_PATH" || v != "/_svc/redisinsight" {
		t.Errorf("PresetProxyEnv = (%q,%q,%v), want (RI_PROXY_PATH, /_svc/redisinsight, true)", k, v, ok)
	}
	// A user custom service (no bundled preset) gets no proxy env.
	if _, _, ok := PresetProxyEnv(&CustomService{Name: "x"}); ok {
		t.Error("non-bundled service must not receive proxy env")
	}
}

func TestMongoExpressProxyEnvInjectedByPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// mongo-express mounts its whole router at site.baseUrl, so the value keeps
	// the trailing slash its config expects. It stays out of the service YAML
	// because it moves the app off "/", and a binary that predates the proxy
	// still opens the dashboard there.
	svc := &CustomService{Name: "mongo-express", Preset: "mongo-express", Dashboard: "http://localhost:8082", DashboardExternal: true}
	k, v, ok := PresetProxyEnv(svc)
	if !ok || k != "ME_CONFIG_SITE_BASEURL" || v != "/_svc/mongo-express/" {
		t.Errorf("PresetProxyEnv = (%q,%q,%v), want (ME_CONFIG_SITE_BASEURL, /_svc/mongo-express/, true)", k, v, ok)
	}
}

// These three ask for the proxy with the flag an older binary ignores.
// dashboard_external is understood by every released binary that proxies, so it
// would route the overlay to /_svc/<name>/ on nothing more than a store
// refresh, before that binary does the rest of what the mount needs, and the
// dashboard would answer 404 in between. pgadmin and mongo-express because the
// path is supplied by the binary, phpmyadmin because its alias only reaches the
// container on a restart that regenerates the unit, which an older reconcile
// does after it has already restarted. Verified on a 1.31.0 guest.
func TestProxiedPresetsAskWithTheInertFlag(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, name := range []string{"phpmyadmin", "pgadmin", "mongo-express"} {
		p, err := LoadPreset(name)
		if err != nil {
			t.Fatalf("LoadPreset(%s): %v", name, err)
		}
		if p.DashboardExternal {
			t.Errorf("%s takes its mount path from the binary, so it must not use dashboard_external", name)
		}
		if !p.DashboardProxy {
			t.Errorf("%s must set dashboard_proxy to be served same-origin", name)
		}
	}
}

// A binary that knows the proxy must not inject the prefix when the store
// preset backing an installed service does not ask to be proxied: the env moves
// the app off "/", while the dashboard link still points there, and the overlay
// gets the upstream's own 404.
func TestPresetProxyPrefix_NotInjectedWhenPresetIsNotProxied(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeStorePreset(t, "mongo-express", "name: mongo-express\nimage: docker.io/library/mongo-express:latest\ndashboard: http://localhost:8082\n")
	svc := &CustomService{Name: "mongo-express", Preset: "mongo-express", Dashboard: "http://localhost:8082"}
	if k, v, ok := PresetProxyEnv(svc); ok {
		t.Errorf("PresetProxyEnv = (%q,%q,true), want no env while the preset keeps the dashboard off the proxy", k, v)
	}
	writeStorePreset(t, "pgadmin", "name: pgadmin\nimage: docker.io/dpage/pgadmin4:latest\ndashboard: http://localhost:8081\n")
	pga := &CustomService{Name: "pgadmin", Preset: "pgadmin", Dashboard: "http://localhost:8081"}
	if k, v, ok := PresetProxyHeader(pga); ok {
		t.Errorf("PresetProxyHeader = (%q,%q,true), want no header while the preset keeps the dashboard off the proxy", k, v)
	}
}

func TestPgAdminProxyHeaderInjectedByPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// pgAdmin reads the mount path per request from X-Script-Name, so the proxy
	// sends it as a header instead of an env var: an install picks the prefix up
	// without a restart, and the app keeps answering at "/" for anything else.
	svc := &CustomService{Name: "pgadmin", Preset: "pgadmin", Dashboard: "http://localhost:8081", DashboardExternal: true}
	k, v, ok := PresetProxyHeader(svc)
	if !ok || k != "X-Script-Name" || v != "/_svc/pgadmin" {
		t.Errorf("PresetProxyHeader = (%q,%q,%v), want (X-Script-Name, /_svc/pgadmin, true)", k, v, ok)
	}
	if _, _, ok := PresetProxyHeader(&CustomService{Name: "x"}); ok {
		t.Error("non-bundled service must not receive a proxy header")
	}
}

func TestPhpMyAdminPresetMountsPathPrefix(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	files := PresetFiles("phpmyadmin")
	var alias, userConfig string
	for _, f := range files {
		switch f.Target {
		case "/etc/apache2/conf-enabled/lerd-path-prefix.conf":
			alias = f.Content
		case "/etc/phpmyadmin/config.user.inc.php":
			userConfig = f.Content
		}
	}
	// An Alias rather than a DocumentRoot move: the app answers under the proxy
	// mount and at "/", so a binary that predates the proxy still reaches it.
	if !strings.Contains(alias, "Alias /_svc/phpmyadmin /var/www/html") {
		t.Errorf("phpmyadmin must alias /_svc/phpmyadmin to its docroot\n%s", alias)
	}
	// The cross-origin workaround stays. A lerd that does not know dashboard_proxy
	// still frames this from its own origin, and without SameSite=None the cookie
	// is not sent at all there. It is inert once the dashboard is same-origin, so
	// dropping it only takes something away from older binaries.
	for _, kept := range []string{"CookieSameSite", "$_SERVER['HTTPS']"} {
		if !strings.Contains(userConfig, kept) {
			t.Errorf("phpmyadmin config must keep %s for a lerd that still frames it cross-origin\n%s", kept, userConfig)
		}
	}
}

func TestRabbitMQDashboardBootstrap_seedsBasicAuth(t *testing.T) {
	svc := &CustomService{
		Name:      "rabbitmq",
		Preset:    "rabbitmq",
		Dashboard: "http://localhost:15672",
		Environment: map[string]string{
			"RABBITMQ_DEFAULT_USER": "root",
			"RABBITMQ_DEFAULT_PASS": "lerd",
		},
	}
	s := PresetDashboardBootstrap(svc)
	// base64("root:lerd") == "cm9vdDpsZXJk"
	for _, want := range []string{"<script>", "rabbitmq.credentials", "cm9vdDpsZXJk", "rabbitmq.auth-scheme", "loggedIn"} {
		if !strings.Contains(s, want) {
			t.Errorf("rabbitmq bootstrap missing %q:\n%s", want, s)
		}
	}
	// A user custom service (no bundled preset) gets no bootstrap.
	if PresetDashboardBootstrap(&CustomService{Name: "x"}) != "" {
		t.Error("non-bundled service must not get a dashboard bootstrap")
	}
}

func TestMySQLPresetContainsCompatDirectives(t *testing.T) {
	files := PresetFiles("mysql")
	if len(files) == 0 {
		t.Fatal("mysql preset has no file mounts")
	}

	cnf := files[0].Content

	for _, directive := range []string{
		"mysql-native-password=ON",
		"restrict-fk-on-non-standard-key=OFF",
	} {
		if !strings.Contains(cnf, directive) {
			t.Errorf("mysql lerd.cnf missing %q", directive)
		}
	}
	// mysql 9.x removed mysql_native_password, so the policy line must not
	// pin it as the primary or the server refuses to initialise.
	if strings.Contains(cnf, "authentication_policy=") {
		t.Errorf("mysql lerd.cnf must not pin authentication_policy: it breaks mysql 9.x init")
	}
}

// Removed in MySQL 8.0; kept silent on 5.7/8.x via the loose- prefix but
// generated a startup warning on every container start. lerd no longer
// ships 5.6, so they should not be re-added.
func TestMySQLPresetExcludesRemovedDirectives(t *testing.T) {
	files := PresetFiles("mysql")
	if len(files) == 0 {
		t.Fatal("mysql preset has no file mounts")
	}

	cnf := files[0].Content

	for _, directive := range []string{
		"innodb_large_prefix",
		"innodb_file_format",
	} {
		if strings.Contains(cnf, directive) {
			t.Errorf("mysql lerd.cnf still contains removed-in-8.0 directive %q", directive)
		}
	}
}

// A console whose build is rooted at a path of its own is served there, not at
// the /_svc/ mount, or its own asset and router paths point nowhere.
func TestDashboardMountPathForOwnPath(t *testing.T) {
	svc := DefaultPresetService("rustfs")
	if svc == nil {
		t.Fatal("no synthesised service for rustfs")
	}
	if !DashboardProxyAtOwnPath(svc) {
		t.Fatal("rustfs should be served at its own path")
	}
	if got := DashboardMountPath(svc); got != "/rustfs/console/" {
		t.Errorf("DashboardMountPath = %q, want /rustfs/console/", got)
	}
	if !DashboardProxyKeepsHost(svc) {
		t.Error("rustfs signs its requests, so the browser's Host must be kept")
	}
	if got := DashboardMounts()["rustfs"]; got != "/rustfs/console/" {
		t.Errorf("DashboardMounts[rustfs] = %q, want /rustfs/console/", got)
	}
}

// A service with no path of its own keeps the /_svc/<name>/ mount.
func TestDashboardMountPathDefault(t *testing.T) {
	svc := DefaultPresetService("mailpit")
	if got := DashboardMountPath(svc); got != DashboardProxyPath("mailpit") {
		t.Errorf("DashboardMountPath = %q, want %q", got, DashboardProxyPath("mailpit"))
	}
	if _, ok := DashboardMounts()["mailpit"]; ok {
		t.Error("mailpit should not claim a path of its own")
	}
}

func TestDashboardLoginScript(t *testing.T) {
	svc := DefaultPresetService("rustfs")
	script := DashboardLoginScript(svc)
	for _, want := range []string{"#accessKey", "#secretKey", "lerd", "lerdpassword", "auth.credentials", "/rustfs/console/auth/login/"} {
		if !strings.Contains(script, want) {
			t.Errorf("login script missing %q", want)
		}
	}
	// The form belongs to a framework that tracks its inputs, so a value has to
	// be announced rather than assigned.
	if !strings.Contains(script, "dispatchEvent") {
		t.Error("login script assigns values without announcing them")
	}
	if DashboardLoginScript(DefaultPresetService("mailpit")) != "" {
		t.Error("a preset that declares no login form should get no script")
	}
}

// A session that has run out leaves its key in place, so the key's presence
// alone cannot stand for being logged in.
func TestDashboardLoginScriptReadsSessionExpiry(t *testing.T) {
	script := DashboardLoginScript(DefaultPresetService("rustfs"))
	for _, want := range []string{`,X="Expiration"`, "Date.parse(e)>Date.now()"} {
		if !strings.Contains(script, want) {
			t.Errorf("login script missing %q", want)
		}
	}
	if n := strings.Count(script, "localStorage.getItem(D)"); n != 1 {
		t.Errorf("the stored key is read in %d places; only the session check should read it", n)
	}
	if opens, closes := strings.Count(script, "{"), strings.Count(script, "}"); opens != closes {
		t.Errorf("login script braces unbalanced: %d open, %d close", opens, closes)
	}
}

// The wrapper has to hand a rebuilt request a body it can send: a Request built
// from a Request carries a stream, which a browser refuses over HTTP/1.1.
func TestDashboardRerouteScriptShape(t *testing.T) {
	script := DashboardRerouteScript("rustfs", "/rustfs/console/")
	for _, want := range []string{"/_svc/rustfs", "/rustfs/console/", "return m+u;}", "arrayBuffer", "XMLHttpRequest.prototype.open"} {
		if !strings.Contains(script, want) {
			t.Errorf("reroute script missing %q", want)
		}
	}
	if opens, closes := strings.Count(script, "{"), strings.Count(script, "}"); opens != closes {
		t.Errorf("reroute script braces unbalanced: %d open, %d close", opens, closes)
	}
}
