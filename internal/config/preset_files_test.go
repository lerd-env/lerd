package config

import (
	"strings"
	"testing"
)

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
