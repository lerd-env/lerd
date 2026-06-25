package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestApplyHostDBExternalEnv_emitsSocketAndMarksExternal(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")() // socket transport is the Linux path
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "mysql"},
		Services: []config.ProjectService{{Name: "mysql"}},
	}
	envMap := map[string]string{"DB_USERNAME": "lerd", "DB_PASSWORD": "secret"}
	extServices := map[string]bool{}
	envOverrides := map[string]string{}

	if !applyHostDBExternalEnv(proj, envMap, extServices, envOverrides) {
		t.Fatal("expected applied=true for a db.external mysql project")
	}
	// Marked external so runEnv skips ensureServiceRunning (no lerd-mysql container).
	if !extServices["mysql"] {
		t.Error(`extServices["mysql"] should be true so lerd-mysql is not started`)
	}
	// Connection vars layered into envOverrides (which win over def.Vars).
	if got := envOverrides["DB_CONNECTION"]; got != "mysql" {
		t.Errorf("DB_CONNECTION = %q, want mysql", got)
	}
	if got := envOverrides["DB_HOST"]; got != "localhost" {
		t.Errorf("DB_HOST = %q, want localhost", got)
	}
	if got := envOverrides["DB_SOCKET"]; got != config.DefaultHostMySQLSocket {
		t.Errorf("DB_SOCKET = %q, want %q", got, config.DefaultHostMySQLSocket)
	}
	// DB_PORT must be present-and-empty (cleared), not absent.
	if v, ok := envOverrides["DB_PORT"]; !ok || v != "" {
		t.Errorf("DB_PORT override = %q present=%v, want present and empty", v, ok)
	}
	// Real host credentials preserved, not clobbered by container defaults.
	if envOverrides["DB_USERNAME"] != "lerd" || envOverrides["DB_PASSWORD"] != "secret" {
		t.Errorf("host creds not preserved: user=%q pass=%q", envOverrides["DB_USERNAME"], envOverrides["DB_PASSWORD"])
	}
}

func TestApplyHostDBExternalEnv_customSocket(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")() // socket transport is the Linux path
	proj := &config.ProjectConfig{DB: config.ProjectDB{External: true, Socket: "/tmp/mysqld.sock"}}
	envOverrides := map[string]string{}
	if !applyHostDBExternalEnv(proj, map[string]string{}, map[string]bool{}, envOverrides) {
		t.Fatal("expected applied=true")
	}
	if got := envOverrides["DB_SOCKET"]; got != "/tmp/mysqld.sock" {
		t.Errorf("DB_SOCKET = %q, want /tmp/mysqld.sock", got)
	}
}

func TestApplyHostDBExternalEnv_macOSEmitsTCP(t *testing.T) {
	defer config.SetHostDBGOOSForTest("darwin")()
	// db.socket is set but must be ignored on macOS — the host socket can't be
	// reached from inside the podman-machine VM, so the connection goes over TCP.
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "mysql", Socket: "/tmp/ignored.sock"},
		Services: []config.ProjectService{{Name: "mysql"}},
	}
	extServices := map[string]bool{}
	envOverrides := map[string]string{}

	if !applyHostDBExternalEnv(proj, map[string]string{}, extServices, envOverrides) {
		t.Fatal("expected applied=true for a db.external mysql project")
	}
	if !extServices["mysql"] {
		t.Error(`extServices["mysql"] should be true so lerd-mysql is not started`)
	}
	if got := envOverrides["DB_HOST"]; got != config.HostDBTCPHost {
		t.Errorf("DB_HOST = %q, want %q (gvproxy host alias)", got, config.HostDBTCPHost)
	}
	if got := envOverrides["DB_PORT"]; got != "3306" {
		t.Errorf("DB_PORT = %q, want 3306 (MySQL canonical port)", got)
	}
	// DB_SOCKET must be present-and-empty (cleared) so a stale socket can't win.
	if v, ok := envOverrides["DB_SOCKET"]; !ok || v != "" {
		t.Errorf("DB_SOCKET override = %q present=%v, want present and empty", v, ok)
	}
}

func TestApplyHostDBExternalEnv_postgresSocketDirContract(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")() // socket transport is the Linux path
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "postgres"},
		Services: []config.ProjectService{{Name: "postgres"}},
	}
	extServices := map[string]bool{}
	envOverrides := map[string]string{}

	if !applyHostDBExternalEnv(proj, map[string]string{}, extServices, envOverrides) {
		t.Fatal("expected applied=true for a db.external postgres project")
	}
	if !extServices["postgres"] {
		t.Error(`extServices["postgres"] should be true so lerd-postgres is not started`)
	}
	// Postgres keeps the pgsql driver.
	if got := envOverrides["DB_CONNECTION"]; got != "pgsql" {
		t.Errorf("DB_CONNECTION = %q, want pgsql", got)
	}
	// Postgres connects over its socket DIRECTORY via DB_HOST (libpq), NOT DB_SOCKET.
	if got := envOverrides["DB_HOST"]; got != "/var/run/postgresql" {
		t.Errorf("DB_HOST = %q, want /var/run/postgresql (socket directory)", got)
	}
	// DB_PORT is retained so libpq forms <dir>/.s.PGSQL.<port>.
	if got := envOverrides["DB_PORT"]; got != "5432" {
		t.Errorf("DB_PORT = %q, want 5432 (retained for the socket filename)", got)
	}
	// DB_SOCKET must be present-and-empty (cleared): pgsql has no unix_socket option.
	if v, ok := envOverrides["DB_SOCKET"]; !ok || v != "" {
		t.Errorf("DB_SOCKET override = %q present=%v, want present and empty", v, ok)
	}
}

func TestApplyHostDBExternalEnv_postgresMacOSEmitsTCP(t *testing.T) {
	defer config.SetHostDBGOOSForTest("darwin")()
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "postgres"},
		Services: []config.ProjectService{{Name: "postgres"}},
	}
	envOverrides := map[string]string{}
	if !applyHostDBExternalEnv(proj, map[string]string{}, map[string]bool{}, envOverrides) {
		t.Fatal("expected applied=true for a db.external postgres project")
	}
	if got := envOverrides["DB_HOST"]; got != config.HostDBTCPHost {
		t.Errorf("DB_HOST = %q, want %q (gvproxy host alias)", got, config.HostDBTCPHost)
	}
	// macOS uses the engine's canonical port — 5432 for Postgres, not 3306.
	if got := envOverrides["DB_PORT"]; got != "5432" {
		t.Errorf("DB_PORT = %q, want 5432 (Postgres canonical port over TCP)", got)
	}
	if got := envOverrides["DB_CONNECTION"]; got != "pgsql" {
		t.Errorf("DB_CONNECTION = %q, want pgsql", got)
	}
	if v, ok := envOverrides["DB_SOCKET"]; !ok || v != "" {
		t.Errorf("DB_SOCKET override = %q present=%v, want present and empty", v, ok)
	}
}

func TestApplyHostDBExternalEnv_noopWhenNotExternal(t *testing.T) {
	proj := &config.ProjectConfig{DB: config.ProjectDB{Service: "mysql"}} // External is false
	extServices := map[string]bool{}
	envOverrides := map[string]string{}
	if applyHostDBExternalEnv(proj, map[string]string{}, extServices, envOverrides) {
		t.Fatal("expected applied=false when db.external is unset")
	}
	if len(extServices) != 0 || len(envOverrides) != 0 {
		t.Errorf("maps must be untouched when not external: ext=%v overrides=%v", extServices, envOverrides)
	}
}

func TestApplyHostDBExternalEnv_marksOnlyOwnFamily(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")()
	// A MySQL host site must NOT mark postgres external. postgres is a default preset
	// that sorts after mysql in the service loop, so marking it would overwrite the
	// site's MySQL connection vars with the postgres preset's (e.g. DB_USERNAME=postgres).
	mysqlProj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "mysql"},
		Services: []config.ProjectService{{Name: "mysql"}},
	}
	ext := map[string]bool{}
	applyHostDBExternalEnv(mysqlProj, map[string]string{}, ext, map[string]string{})
	if ext["postgres"] {
		t.Error("MySQL host site must NOT mark postgres external (would leak pgsql preset env)")
	}
	if !ext["mysql"] || !ext["mariadb"] {
		t.Error("MySQL host site should mark mysql+mariadb external")
	}

	// A Postgres host site must NOT mark mysql/mariadb external.
	pgProj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "postgres"},
		Services: []config.ProjectService{{Name: "postgres"}},
	}
	ext = map[string]bool{}
	applyHostDBExternalEnv(pgProj, map[string]string{}, ext, map[string]string{})
	if ext["mysql"] || ext["mariadb"] {
		t.Error("Postgres host site must NOT mark mysql/mariadb external")
	}
	if !ext["postgres"] {
		t.Error("Postgres host site should mark postgres external")
	}
}

func TestApplyHostDBExternalEnv_postgresHonoursPortOverride(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")()
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "postgres"},
		Services: []config.ProjectService{{Name: "postgres"}},
	}
	// A user committed DB_PORT=5433 via .env.lerd_override (already layered into
	// envOverrides). For Postgres the port forms the socket filename (.s.PGSQL.5433),
	// so it must be preserved, not clobbered back to the 5432 default.
	envOverrides := map[string]string{"DB_PORT": "5433"}
	applyHostDBExternalEnv(proj, map[string]string{}, map[string]bool{}, envOverrides)
	if got := envOverrides["DB_PORT"]; got != "5433" {
		t.Errorf("DB_PORT = %q, want 5433 (user override preserved for the socket filename)", got)
	}
}

func TestApplyHostDBExternalEnv_skipsNonHostCapableFamily(t *testing.T) {
	// Redis has no host backend, so db.external is a no-op for it (unlike MySQL,
	// MariaDB, and Postgres, which are all host-capable).
	proj := &config.ProjectConfig{
		DB:       config.ProjectDB{External: true, Service: "redis"},
		Services: []config.ProjectService{{Name: "redis"}},
	}
	if applyHostDBExternalEnv(proj, map[string]string{}, map[string]bool{}, map[string]string{}) {
		t.Fatal("expected applied=false for redis — it has no host backend")
	}
}

func TestApplyHostDBExternalEnv_doesNotInventAbsentCreds(t *testing.T) {
	// When .env carries no DB_USERNAME/DB_PASSWORD, the helper must not set them,
	// so the framework defaults still apply rather than empty overrides.
	proj := &config.ProjectConfig{DB: config.ProjectDB{External: true, Service: "mysql"}}
	envOverrides := map[string]string{}
	applyHostDBExternalEnv(proj, map[string]string{}, map[string]bool{}, envOverrides)
	if _, ok := envOverrides["DB_USERNAME"]; ok {
		t.Error("DB_USERNAME should be absent when not present in .env")
	}
	if _, ok := envOverrides["DB_PASSWORD"]; ok {
		t.Error("DB_PASSWORD should be absent when not present in .env")
	}
}

func TestApplyHostDBExternalEnv_nilProject(t *testing.T) {
	if applyHostDBExternalEnv(nil, map[string]string{}, map[string]bool{}, map[string]string{}) {
		t.Fatal("expected applied=false for a nil project (no panic)")
	}
}

func TestHostDBSetupNotes(t *testing.T) {
	mysql, ok := config.HostBackendFor("mysql")
	if !ok {
		t.Fatal("no mysql host-backend spec")
	}
	postgres, ok := config.HostBackendFor("postgres")
	if !ok {
		t.Fatal("no postgres host-backend spec")
	}
	join := func(lines []string) string { return strings.Join(lines, "\n") }

	t.Run("linux mysql: socket line, no auth note", func(t *testing.T) {
		out := join(hostDBSetupNotes(mysql, false, "/run/mysqld/mysqld.sock", "lerd"))
		if !strings.Contains(out, "connecting via socket /run/mysqld/mysqld.sock") {
			t.Errorf("missing socket transport line:\n%s", out)
		}
		// MySQL over the socket authenticates by user+password, so no auth caveat.
		if strings.Contains(out, "pg_hba") || strings.Contains(out, "gvproxy") {
			t.Errorf("mysql socket path should carry no pg_hba/gvproxy note:\n%s", out)
		}
	})

	t.Run("linux postgres: pg_hba peer-auth note", func(t *testing.T) {
		out := join(hostDBSetupNotes(postgres, false, "/var/run/postgresql", "appuser"))
		if !strings.Contains(out, "connecting via socket /var/run/postgresql") {
			t.Errorf("missing socket transport line:\n%s", out)
		}
		if !strings.Contains(out, "local all appuser scram-sha-256") {
			t.Errorf("missing pg_hba peer-auth note carrying the user:\n%s", out)
		}
	})

	t.Run("darwin mysql: TCP grant/bind note", func(t *testing.T) {
		out := join(hostDBSetupNotes(mysql, true, "", "appuser"))
		if !strings.Contains(out, "connecting via TCP "+config.HostDBTCPHost) {
			t.Errorf("missing TCP transport line:\n%s", out)
		}
		if !strings.Contains(out, "non-loopback source") {
			t.Errorf("missing gvproxy non-loopback explanation:\n%s", out)
		}
		if !strings.Contains(out, "grant appuser on '%'") {
			t.Errorf("missing mysql grant-on-%% note:\n%s", out)
		}
		if strings.Contains(out, "listen_addresses") {
			t.Errorf("mysql note must not mention postgres listen_addresses:\n%s", out)
		}
	})

	t.Run("darwin postgres: TCP listen_addresses + host-line note", func(t *testing.T) {
		out := join(hostDBSetupNotes(postgres, true, "", "appuser"))
		if !strings.Contains(out, "connecting via TCP "+config.HostDBTCPHost) {
			t.Errorf("missing TCP transport line:\n%s", out)
		}
		if !strings.Contains(out, "listen_addresses") {
			t.Errorf("missing listen_addresses note:\n%s", out)
		}
		if !strings.Contains(out, "host all appuser") {
			t.Errorf("missing pg_hba host-line note carrying the user:\n%s", out)
		}
	})

	t.Run("empty user falls back to a placeholder", func(t *testing.T) {
		out := join(hostDBSetupNotes(postgres, false, "/var/run/postgresql", ""))
		if !strings.Contains(out, "local all <db-user> scram-sha-256") {
			t.Errorf("empty user should fall back to <db-user>:\n%s", out)
		}
	})
}

func TestClearStaleHostDBSocket_clearsOnToggleOff(t *testing.T) {
	// A project that was in host MySQL mode (.env still carries the host socket) is
	// switched back to container mode (db.external removed). The stale DB_SOCKET must be
	// cleared via envOverrides so the container is used, not a now-dead host socket.
	proj := &config.ProjectConfig{DB: config.ProjectDB{Service: "mysql"}} // External is false
	envMap := map[string]string{"DB_SOCKET": config.DefaultHostMySQLSocket}
	envOverrides := map[string]string{}
	if !clearStaleHostDBSocket(proj, envMap, envOverrides) {
		t.Fatal("expected cleared=true for a host-capable container-mode project carrying a stale socket")
	}
	if v, ok := envOverrides["DB_SOCKET"]; !ok || v != "" {
		t.Errorf("DB_SOCKET override = %q present=%v, want present and empty", v, ok)
	}
}

func TestClearStaleHostDBSocket_noopWhenStillExternal(t *testing.T) {
	// While db.external is on, host mode owns DB_SOCKET — the clear must not fire.
	proj := &config.ProjectConfig{DB: config.ProjectDB{External: true, Service: "mysql"}}
	envOverrides := map[string]string{}
	if clearStaleHostDBSocket(proj, map[string]string{"DB_SOCKET": config.DefaultHostMySQLSocket}, envOverrides) {
		t.Fatal("expected noop while still external")
	}
	if _, ok := envOverrides["DB_SOCKET"]; ok {
		t.Error("must not touch DB_SOCKET while external")
	}
}

func TestClearStaleHostDBSocket_respectsUserOverride(t *testing.T) {
	// A user who intentionally set DB_SOCKET via .env.lerd_override (already layered into
	// envOverrides) keeps it — lerd only clears the socket it wrote itself.
	proj := &config.ProjectConfig{DB: config.ProjectDB{Service: "mysql"}}
	envOverrides := map[string]string{"DB_SOCKET": "/tmp/custom.sock"}
	if clearStaleHostDBSocket(proj, map[string]string{"DB_SOCKET": config.DefaultHostMySQLSocket}, envOverrides) {
		t.Fatal("expected noop when the user re-asserted a socket via .env.lerd_override")
	}
	if envOverrides["DB_SOCKET"] != "/tmp/custom.sock" {
		t.Errorf("user socket override must be preserved, got %q", envOverrides["DB_SOCKET"])
	}
}

func TestClearStaleHostDBSocket_noopWhenNoStaleSocket(t *testing.T) {
	// Nothing to clear → don't add a noisy empty DB_SOCKET to a fresh container project.
	proj := &config.ProjectConfig{DB: config.ProjectDB{Service: "mysql"}}
	envOverrides := map[string]string{}
	if clearStaleHostDBSocket(proj, map[string]string{}, envOverrides) {
		t.Fatal("expected noop when .env has no DB_SOCKET to clear")
	}
	if _, ok := envOverrides["DB_SOCKET"]; ok {
		t.Error("must not add an empty DB_SOCKET when there was nothing to clear")
	}
}

func TestClearStaleHostDBSocket_noopForNonHostCapable(t *testing.T) {
	// Redis is not a host-capable DB family, so the socket-clear logic never applies.
	proj := &config.ProjectConfig{DB: config.ProjectDB{Service: "redis"}}
	envOverrides := map[string]string{}
	if clearStaleHostDBSocket(proj, map[string]string{"DB_SOCKET": "/x"}, envOverrides) {
		t.Fatal("expected noop for a non-host-capable family")
	}
}
