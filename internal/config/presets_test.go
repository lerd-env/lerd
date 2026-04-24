package config

import (
	"strings"
	"testing"
)

func TestListPresets_IncludesShippedPresets(t *testing.T) {
	presets, err := ListPresets()
	if err != nil {
		t.Fatalf("ListPresets() error = %v", err)
	}
	want := map[string]bool{
		"phpmyadmin":    false,
		"pgadmin":       false,
		"mongo":         false,
		"mongo-express": false,
		"selenium":      false,
		"stripe-mock":   false,
		"mysql":         false,
		"memcached":     false,
		"rabbitmq":      false,
		"elasticsearch": false,
		"elasticvue":    false,
	}
	for _, p := range presets {
		if _, ok := want[p.Name]; ok {
			want[p.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ListPresets() missing bundled preset %q", name)
		}
	}
}

func TestListPresets_SortedByName(t *testing.T) {
	presets, err := ListPresets()
	if err != nil {
		t.Fatalf("ListPresets() error = %v", err)
	}
	for i := 1; i < len(presets); i++ {
		if presets[i-1].Name > presets[i].Name {
			t.Errorf("ListPresets() not sorted: %q > %q", presets[i-1].Name, presets[i].Name)
		}
	}
}

func TestLoadPreset_PhpMyAdmin(t *testing.T) {
	p, err := LoadPreset("phpmyadmin")
	if err != nil {
		t.Fatalf("LoadPreset(phpmyadmin) error = %v", err)
	}
	if p.Name != "phpmyadmin" || p.Image == "" || len(p.Ports) == 0 || p.Dashboard == "" {
		t.Errorf("phpmyadmin preset missing required fields: %+v", p)
	}
	if len(p.DependsOn) != 1 || p.DependsOn[0] != "mysql" {
		t.Errorf("phpmyadmin should depend on mysql, got %v", p.DependsOn)
	}
	foundFramingCfg := false
	for _, f := range PresetFiles("phpmyadmin") {
		if f.Target == "/etc/phpmyadmin/config.user.inc.php" && strings.Contains(f.Content, "AllowThirdPartyFraming") {
			foundFramingCfg = true
			break
		}
	}
	if !foundFramingCfg {
		t.Errorf("phpmyadmin preset must ship config.user.inc.php enabling AllowThirdPartyFraming for iframe embedding")
	}
}

func TestLoadPreset_PgAdmin(t *testing.T) {
	p, err := LoadPreset("pgadmin")
	if err != nil {
		t.Fatalf("LoadPreset(pgadmin) error = %v", err)
	}
	if len(p.DependsOn) != 1 || p.DependsOn[0] != "postgres" {
		t.Errorf("pgadmin should depend on postgres, got %v", p.DependsOn)
	}
	foundFramingCfg := false
	for _, f := range PresetFiles("pgadmin") {
		if f.Target == "/pgadmin4/config_local.py" && strings.Contains(f.Content, "X_FRAME_OPTIONS") {
			foundFramingCfg = true
			break
		}
	}
	if !foundFramingCfg {
		t.Errorf("pgadmin preset must ship config_local.py clearing X_FRAME_OPTIONS for iframe embedding")
	}
}

func TestLoadPreset_Memcached(t *testing.T) {
	p, err := LoadPreset("memcached")
	if err != nil {
		t.Fatalf("LoadPreset(memcached) error = %v", err)
	}
	if p.Image == "" || len(p.Ports) != 1 || p.Ports[0] != "11211:11211" {
		t.Errorf("memcached preset missing image or 11211:11211 port, got: %+v", p)
	}
	if p.DataDir != "" {
		t.Errorf("memcached is in-memory and must not declare data_dir, got %q", p.DataDir)
	}
	if p.EnvDetect == nil || p.EnvDetect.Key != "MEMCACHED_HOST" {
		t.Errorf("memcached env_detect should be key=MEMCACHED_HOST, got %+v", p.EnvDetect)
	}
}

func TestLoadPreset_RabbitMQ(t *testing.T) {
	p, err := LoadPreset("rabbitmq")
	if err != nil {
		t.Fatalf("LoadPreset(rabbitmq) error = %v", err)
	}
	if len(p.Ports) != 2 {
		t.Errorf("rabbitmq should publish AMQP (5672) and management UI (15672), got %v", p.Ports)
	}
	if p.Dashboard == "" {
		t.Errorf("rabbitmq should expose the management UI as dashboard")
	}
	if p.DataDir == "" {
		t.Errorf("rabbitmq should persist /var/lib/rabbitmq for queue durability across restarts")
	}
	if !p.DashboardExternal {
		t.Errorf("rabbitmq must set dashboard_external because Cowboy's session cookie can't be carried by the iframe")
	}
}

func TestLoadPreset_Elasticvue(t *testing.T) {
	p, err := LoadPreset("elasticvue")
	if err != nil {
		t.Fatalf("LoadPreset(elasticvue) error = %v", err)
	}
	if len(p.DependsOn) != 1 || p.DependsOn[0] != "elasticsearch" {
		t.Errorf("elasticvue should depend on elasticsearch, got %v", p.DependsOn)
	}
	if p.Dashboard == "" {
		t.Errorf("elasticvue must expose its UI as dashboard")
	}
	if got := p.Environment["ELASTICVUE_CLUSTERS"]; got == "" {
		t.Errorf("elasticvue must pre-configure the lerd ES cluster via ELASTICVUE_CLUSTERS")
	}
}

func TestLoadPreset_ElasticsearchEnablesCors(t *testing.T) {
	p, err := LoadPreset("elasticsearch")
	if err != nil {
		t.Fatalf("LoadPreset(elasticsearch) error = %v", err)
	}
	if p.Environment["http.cors.enabled"] != "true" {
		t.Errorf("elasticsearch must enable CORS so the elasticvue browser SPA can reach it, got %q", p.Environment["http.cors.enabled"])
	}
	// The wildcard must be wrapped in literal quotes because ES parses env
	// vars as YAML and a bare '*' becomes an alias token that crashes the
	// SnakeYAML scanner on boot.
	if p.Environment["http.cors.allow-origin"] != `"*"` {
		t.Errorf(`elasticsearch must allow any origin for local dev (quoted to survive YAML parse), got %q`, p.Environment["http.cors.allow-origin"])
	}
}

func TestLoadPreset_Elasticsearch(t *testing.T) {
	p, err := LoadPreset("elasticsearch")
	if err != nil {
		t.Fatalf("LoadPreset(elasticsearch) error = %v", err)
	}
	if p.Environment["discovery.type"] != "single-node" {
		t.Errorf("elasticsearch must run in single-node mode for local dev (skips production bootstrap checks), got %q", p.Environment["discovery.type"])
	}
	if p.Environment["xpack.security.enabled"] != "false" {
		t.Errorf("elasticsearch must disable xpack security so apps can connect without TLS+auth in dev, got %q", p.Environment["xpack.security.enabled"])
	}
	if p.EnvDetect == nil || p.EnvDetect.Composer != "elasticsearch/elasticsearch" {
		t.Errorf("elasticsearch env_detect should fire on composer elasticsearch/elasticsearch, got %+v", p.EnvDetect)
	}
	if p.Userns != "keep-id:uid=1000,gid=0" {
		t.Errorf("elasticsearch must map host user to container UID 1000 via keep-id, got %q", p.Userns)
	}
	if !p.ChownData {
		t.Errorf("elasticsearch must set chown_data so the host data dir is owned by the container UID at mount time")
	}
}

func TestLoadPreset_MySQL_MultiVersion(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("LoadPreset(mysql) error = %v", err)
	}
	if p.Image != "" {
		t.Errorf("multi-version preset must not declare top-level image, got %q", p.Image)
	}
	if len(p.Versions) < 2 {
		t.Errorf("expected at least 2 versions, got %d", len(p.Versions))
	}
	if p.DefaultVersion == "" {
		t.Errorf("DefaultVersion should be set (defaults to first version)")
	}
}

func TestPresetResolve_MultiVersion(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("LoadPreset(mysql) error = %v", err)
	}
	svc, err := p.Resolve("5.7")
	if err != nil {
		t.Fatalf("Resolve(5.7): %v", err)
	}
	if svc.Name != "mysql-5-7" {
		t.Errorf("Name = %q, want mysql-5-7", svc.Name)
	}
	if svc.Image != "docker.io/library/mysql:5.7" {
		t.Errorf("Image = %q, want docker.io/library/mysql:5.7", svc.Image)
	}
	foundHost := false
	for _, kv := range svc.EnvVars {
		if kv == "DB_HOST=lerd-mysql-5-7" {
			foundHost = true
		}
	}
	if !foundHost {
		t.Errorf("expected DB_HOST=lerd-mysql-5-7 in env_vars, got %v", svc.EnvVars)
	}
}

func TestPresetResolve_DefaultVersion(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	svc, err := p.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	}
	if svc.Name != "mysql-"+SanitizeImageTag(p.DefaultVersion) {
		t.Errorf("Resolve(\"\") should fall back to DefaultVersion, got Name=%q", svc.Name)
	}
}

func TestPresetResolve_UnknownVersion(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	if _, err := p.Resolve("9.9"); err == nil {
		t.Errorf("Resolve(9.9) should error for unknown version")
	}
}

func TestServicesInFamily_BuiltinAndCustom(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	// Built-in mysql is always in family "mysql".
	hosts := ServicesInFamily("mysql")
	if len(hosts) != 1 || hosts[0] != "lerd-mysql" {
		t.Errorf("expected [lerd-mysql], got %v", hosts)
	}

	// Install a fake mysql alternate.
	alt := &CustomService{
		Name:   "mysql-5-7",
		Image:  "docker.io/library/mysql:5.7",
		Family: "mysql",
	}
	if err := SaveCustomService(alt); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	hosts = ServicesInFamily("mysql")
	if len(hosts) != 2 || hosts[0] != "lerd-mysql" || hosts[1] != "lerd-mysql-5-7" {
		t.Errorf("expected [lerd-mysql lerd-mysql-5-7], got %v", hosts)
	}
}

func TestResolveDynamicEnv_DiscoverFamily(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	svc := &CustomService{
		Name:  "phpmyadmin",
		Image: "phpmyadmin:latest",
		DynamicEnv: map[string]string{
			"PMA_HOSTS": "discover_family:mysql",
		},
	}
	if err := ResolveDynamicEnv(svc); err != nil {
		t.Fatalf("ResolveDynamicEnv: %v", err)
	}
	if got := svc.Environment["PMA_HOSTS"]; got != "lerd-mysql" {
		t.Errorf("PMA_HOSTS = %q, want lerd-mysql", got)
	}
}

func TestResolveDynamicEnv_RepeatFamily(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	svc := &CustomService{
		Name:  "phpmyadmin",
		Image: "phpmyadmin:latest",
		DynamicEnv: map[string]string{
			"PMA_HOSTS":     "discover_family:mysql,mariadb",
			"PMA_USERS":     "repeat_family:mysql,mariadb=root",
			"PMA_PASSWORDS": "repeat_family:mysql,mariadb=lerd",
		},
	}
	if err := ResolveDynamicEnv(svc); err != nil {
		t.Fatalf("ResolveDynamicEnv: %v", err)
	}
	hosts := strings.Split(svc.Environment["PMA_HOSTS"], ",")
	users := strings.Split(svc.Environment["PMA_USERS"], ",")
	passes := strings.Split(svc.Environment["PMA_PASSWORDS"], ",")
	if len(hosts) == 0 {
		t.Fatalf("expected at least one host, got %q", svc.Environment["PMA_HOSTS"])
	}
	if len(users) != len(hosts) || len(passes) != len(hosts) {
		t.Errorf("users/passwords length mismatch: hosts=%d users=%d passes=%d",
			len(hosts), len(users), len(passes))
	}
	for _, u := range users {
		if u != "root" {
			t.Errorf("user = %q, want root", u)
		}
	}
	for _, p := range passes {
		if p != "lerd" {
			t.Errorf("password = %q, want lerd", p)
		}
	}
}

func TestResolveDynamicEnv_UnknownDirective(t *testing.T) {
	svc := &CustomService{
		Name: "x",
		DynamicEnv: map[string]string{
			"FOO": "garbage:bar",
		},
	}
	if err := ResolveDynamicEnv(svc); err == nil {
		t.Errorf("expected error for unknown directive")
	}
}

func TestSanitizeImageTag(t *testing.T) {
	cases := map[string]string{
		"5.7":        "5-7",
		"8.0.34":     "8-0-34",
		"11.4-focal": "11-4-focal",
		"v1.7":       "v1-7",
		"latest":     "latest",
		"--weird--":  "weird",
	}
	for in, want := range cases {
		if got := SanitizeImageTag(in); got != want {
			t.Errorf("SanitizeImageTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadPreset_Selenium(t *testing.T) {
	p, err := LoadPreset("selenium")
	if err != nil {
		t.Fatalf("LoadPreset(selenium) error = %v", err)
	}
	if p.Name != "selenium" || p.Image == "" || len(p.Ports) == 0 || p.Dashboard == "" {
		t.Errorf("selenium preset missing required fields: %+v", p)
	}
	if !p.ShareHosts {
		t.Error("selenium preset should have share_hosts: true")
	}
}

func TestLoadPreset_Unknown(t *testing.T) {
	if _, err := LoadPreset("does-not-exist"); err == nil {
		t.Errorf("LoadPreset(does-not-exist) expected error, got nil")
	}
}

func TestPresetExists(t *testing.T) {
	if !PresetExists("phpmyadmin") {
		t.Errorf("PresetExists(phpmyadmin) = false, want true")
	}
	if !PresetExists("pgadmin") {
		t.Errorf("PresetExists(pgadmin) = false, want true")
	}
	if PresetExists("nope") {
		t.Errorf("PresetExists(nope) = true, want false")
	}
}
