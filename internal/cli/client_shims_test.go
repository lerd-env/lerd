package cli

import (
	"reflect"
	"testing"
)

// resolveLoopbackTarget owns the whole "did the caller name a host" answer, so
// every shape counts: the -h/--host flag forms, a connection URI, and a libpq
// conninfo string. A shape it fails to see reads as no host at all, and the
// caller then treats a connection that was never ours as a local one, prepending
// -h lerd-<service> and handing it lerd's own credentials.
func TestResolveLoopbackTargetHostDetection(t *testing.T) {
	prevFn := loopbackServiceOwningPortFn
	t.Cleanup(func() { loopbackServiceOwningPortFn = prevFn })
	loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
		t.Fatalf("must not be called: no case here names a loopback host")
		return "", false
	}

	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"mydb"}, false},
		{[]string{"-U", "postgres", "mydb"}, false},
		{[]string{"-h", "db.example.com", "mydb"}, true},
		{[]string{"--host", "db.example.com"}, true},
		{[]string{"--host=db.example.com"}, true},
		{[]string{"-hdb.example.com"}, true},
		{[]string{"--help"}, false},
		{[]string{"postgresql://user@ext.example.com/db"}, true},
		{[]string{"host=ext.example.com dbname=prod"}, true},
		{[]string{"dbname=prod host=ext.example.com"}, true},
		{[]string{"mydb"}, false},
		// A host= or a URL inside a -c/-e SQL body must NOT be read as a host, or
		// a legitimate query fails to connect to the lerd service.
		{[]string{"-c", "SELECT * FROM logs WHERE host='node1'"}, false},
		{[]string{"-e", "UPDATE s SET url='http://x' WHERE id=1"}, false},
		{[]string{"--command", "SELECT 'host=' || h FROM t"}, false},
		// The inline flag form (one argv token) must be handled the same way.
		{[]string{"-eSELECT * FROM logs WHERE host='node1'"}, false},
		{[]string{"--command=SELECT * FROM t WHERE host='x'"}, false},
		// -c/-e are boolean flags for some tools (mysqldump --complete-insert),
		// so a following -h is a real host and must still be detected.
		{[]string{"-c", "-h", "prod.example.com", "mydb"}, true},
		{[]string{"-e", "-h", "prod.example.com"}, true},
		// A spaceless "--where=host=x" filter: its only key is --where, not host, so
		// it must not be read as a conninfo host and suppress the local default.
		{[]string{"mysqldump", "db", "t", "--where=host=x"}, false},
		// A -c/-e body that merely starts with "-h" is not a glued host flag: the
		// real glued form ("-hHOST") carries no whitespace.
		{[]string{"-c", "-h note"}, false},
	}
	for _, c := range cases {
		args, hostGiven, prefer := resolveLoopbackTarget("psql", c.args)
		if hostGiven != c.want {
			t.Errorf("resolveLoopbackTarget(psql, %v) hostGiven = %v, want %v", c.args, hostGiven, c.want)
		}
		// None of these is a loopback match, so the args must come back untouched
		// and pick no local service to route to.
		if prefer != "" || !reflect.DeepEqual(args, c.args) {
			t.Errorf("resolveLoopbackTarget(psql, %v) = (%v, _, %q), want the args unchanged and no prefer", c.args, args, prefer)
		}
	}
}

func TestIsPostgresTool(t *testing.T) {
	for _, tool := range []string{"psql", "pg_dump", "pg_dumpall", "pg_restore"} {
		if !isPostgresTool(tool) {
			t.Errorf("isPostgresTool(%q) = false, want true", tool)
		}
	}
	for _, tool := range []string{"mysql", "mysqldump", "mariadb-dump"} {
		if isPostgresTool(tool) {
			t.Errorf("isPostgresTool(%q) = true, want false", tool)
		}
	}
}

func TestIsSQLTool(t *testing.T) {
	for _, tool := range []string{"mysql", "mysqldump", "psql", "pg_dump", "pg_restore"} {
		if !isSQLTool(tool) {
			t.Errorf("isSQLTool(%q) = false, want true", tool)
		}
	}
	for _, tool := range []string{"redis-cli", "valkey-cli", "mongosh", "mongodump"} {
		if isSQLTool(tool) {
			t.Errorf("isSQLTool(%q) = true, want false", tool)
		}
	}
}

func TestLocalCredsEnv(t *testing.T) {
	pg := localCredsEnv("pg_dump")
	if len(pg) != 2 || pg[0] != "PGUSER=postgres" || pg[1] != "PGPASSWORD=lerd" {
		t.Errorf("postgres creds = %v", pg)
	}
	my := localCredsEnv("mysqldump")
	if len(my) != 1 || my[0] != "MYSQL_PWD=lerd" {
		t.Errorf("mysql creds = %v", my)
	}
}

func TestPathUnder(t *testing.T) {
	cases := []struct {
		path, base string
		want       bool
	}{
		{"/home/u", "/home/u", true},
		{"/home/u/proj", "/home/u", true},
		{"/home/u/", "/home/u", true},
		{"/home/us", "/home/u", false},
		{"/tmp/x", "/home/u", false},
		{"/home/u/proj", "/home/u/", true},
	}
	for _, c := range cases {
		if got := pathUnder(c.path, c.base); got != c.want {
			t.Errorf("pathUnder(%q,%q) = %v, want %v", c.path, c.base, got, c.want)
		}
	}
}

func TestHostEnvSet(t *testing.T) {
	t.Setenv("PGHOST", "")
	t.Setenv("MYSQL_HOST", "")
	if hostEnvSet("pg_dump") || hostEnvSet("mysqldump") {
		t.Fatal("no host env should read as unset")
	}
	t.Setenv("PGHOST", "db.example.com")
	if !hostEnvSet("pg_dump") {
		t.Error("PGHOST should count for postgres tools")
	}
	if hostEnvSet("mysqldump") {
		t.Error("PGHOST must not count for mysql tools")
	}
	t.Setenv("MYSQL_HOST", "db.example.com")
	if !hostEnvSet("mysqldump") {
		t.Error("MYSQL_HOST should count for mysql tools")
	}
}

// A stray PGHOST/MYSQL_HOST left set in the shell must never re-flag a call
// resolveLoopbackTarget already matched to a local lerd service (prefer !=
// "") as external: the -h flag that produced that match is what a real
// client honours over the environment variable, not the other way round.
func TestEffectiveHostGiven(t *testing.T) {
	t.Setenv("PGHOST", "db.example.com")
	t.Setenv("MYSQL_HOST", "")

	if got := effectiveHostGiven("psql", false, "postgres-timescaledb"); got {
		t.Error("a successful loopback match must not be overridden by an unrelated PGHOST")
	}
	if got := effectiveHostGiven("psql", true, ""); !got {
		t.Error("an external -h host must stay external regardless of env")
	}
	if got := effectiveHostGiven("psql", false, ""); !got {
		t.Error("no -h flag at all should still fall back to PGHOST")
	}

	t.Setenv("PGHOST", "")
	if got := effectiveHostGiven("psql", false, ""); got {
		t.Error("no host anywhere (args or env) should read as unset")
	}
}

func TestIsVersionProbe(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--version"}, true},
		{[]string{"-V"}, true},
		{[]string{"--help"}, true},
		{[]string{"-?"}, true},
		{[]string{"mydb"}, false},
		{[]string{"-U", "postgres", "mydb"}, false},
		// Only the leading position counts: that is the one the postgres tools
		// answer, and anywhere else it is a value, not a query.
		{[]string{"-c", "--version"}, false},
		{[]string{"--where=--version"}, false},
	}
	for _, c := range cases {
		if got := isVersionProbe(c.args); got != c.want {
			t.Errorf("isVersionProbe(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestWantsLocalDefault(t *testing.T) {
	cases := []struct {
		tool      string
		args      []string
		hostGiven bool
		want      bool
	}{
		{"pg_dump", []string{"mydb"}, false, true},
		{"mysqldump", []string{"mydb"}, false, true},
		{"pg_dump", []string{"mydb"}, true, false},
		{"mongodump", []string{"mydb"}, false, false},
		// An IDE validates the executable with a bare --version. Prepending the
		// local -h in front of it makes pg_dump exit non-zero with no version.
		{"pg_dump", []string{"--version"}, false, false},
		{"pg_restore", []string{"--help"}, false, false},
		{"mysqldump", []string{"-V"}, false, false},
	}
	for _, c := range cases {
		if got := wantsLocalDefault(c.tool, c.args, c.hostGiven); got != c.want {
			t.Errorf("wantsLocalDefault(%q, %v, %v) = %v, want %v", c.tool, c.args, c.hostGiven, got, c.want)
		}
	}
}

func TestHostFlagSpan(t *testing.T) {
	cases := []struct {
		args             []string
		wantValue        string
		wantStart, wantN int
		wantOK           bool
	}{
		{nil, "", 0, 0, false},
		{[]string{"mydb"}, "", 0, 0, false},
		{[]string{"-h", "127.0.0.1", "-U", "postgres"}, "127.0.0.1", 0, 2, true},
		{[]string{"-U", "postgres", "--host", "db.example.com"}, "db.example.com", 2, 2, true},
		{[]string{"--host=db.example.com"}, "db.example.com", 0, 1, true},
		{[]string{"-hdb.example.com"}, "db.example.com", 0, 1, true},
		// A trailing bare -h with no value is not a usable span.
		{[]string{"-U", "postgres", "-h"}, "", 0, 0, false},
	}
	for _, c := range cases {
		value, start, n, ok := hostFlagSpan(c.args)
		if value != c.wantValue || start != c.wantStart || n != c.wantN || ok != c.wantOK {
			t.Errorf("hostFlagSpan(%v) = (%q, %d, %d, %v), want (%q, %d, %d, %v)",
				c.args, value, start, n, ok, c.wantValue, c.wantStart, c.wantN, c.wantOK)
		}
	}
}

func TestPortFlagSpan(t *testing.T) {
	cases := []struct {
		tool             string
		args             []string
		wantValue        string
		wantStart, wantN int
		wantOK           bool
	}{
		{"psql", []string{"-p", "5433", "-U", "postgres"}, "5433", 0, 2, true},
		{"psql", []string{"--port=5433"}, "5433", 0, 1, true},
		{"psql", []string{"-p5433"}, "5433", 0, 1, true},
		{"psql", nil, "", 0, 0, false},
		// mysql's lowercase -p is --password, not --port, and must never be
		// mistaken for one (which would strip a real password off the args).
		{"mysql", []string{"-psecret", "mydb"}, "", 0, 0, false},
		{"mysql", []string{"-P", "3307", "mydb"}, "3307", 0, 2, true},
		{"mysql", []string{"--port=3307"}, "3307", 0, 1, true},
		{"mysqldump", []string{"-P3307"}, "3307", 0, 1, true},
	}
	for _, c := range cases {
		value, start, n, ok := portFlagSpan(c.tool, c.args)
		if value != c.wantValue || start != c.wantStart || n != c.wantN || ok != c.wantOK {
			t.Errorf("portFlagSpan(%q, %v) = (%q, %d, %d, %v), want (%q, %d, %d, %v)",
				c.tool, c.args, value, start, n, ok, c.wantValue, c.wantStart, c.wantN, c.wantOK)
		}
	}
}

func TestDefaultToolPort(t *testing.T) {
	if got := defaultToolPort("psql"); got != "5432" {
		t.Errorf("defaultToolPort(psql) = %q, want 5432", got)
	}
	if got := defaultToolPort("pg_dump"); got != "5432" {
		t.Errorf("defaultToolPort(pg_dump) = %q, want 5432", got)
	}
	if got := defaultToolPort("mysql"); got != "3306" {
		t.Errorf("defaultToolPort(mysql) = %q, want 3306", got)
	}
	if got := defaultToolPort("mysqldump"); got != "3306" {
		t.Errorf("defaultToolPort(mysqldump) = %q, want 3306", got)
	}
}

// resolveLoopbackTarget is the fix for the bug where `psql -h 127.0.0.1 -p
// 5433 ...` — the normal way to reach a port lerd itself published — always
// failed with connection refused: runClientExec execs the client tool inside
// a throwaway container on its own network, where 127.0.0.1 is the
// container's own loopback, not the host's. It must recognise a loopback
// host whose port matches an installed lerd service and route it like a
// hostless call, while leaving a genuinely external host (or a loopback host
// matching no lerd service) untouched.
func TestResolveLoopbackTarget(t *testing.T) {
	prevFn := loopbackServiceOwningPortFn
	t.Cleanup(func() { loopbackServiceOwningPortFn = prevFn })

	t.Run("no host in args passes through as hostless", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			t.Fatalf("must not be called when no host is given")
			return "", false
		}
		args, hostGiven, prefer := resolveLoopbackTarget("psql", []string{"mydb"})
		if hostGiven || prefer != "" || !reflect.DeepEqual(args, []string{"mydb"}) {
			t.Errorf("got (%v, %v, %q), want (%v, false, \"\")", args, hostGiven, prefer, []string{"mydb"})
		}
	})

	t.Run("external host passes through unchanged", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			t.Fatalf("must not be called for a non-loopback host")
			return "", false
		}
		in := []string{"-h", "db.example.com", "-U", "postgres", "mydb"}
		args, hostGiven, prefer := resolveLoopbackTarget("psql", in)
		if !hostGiven || prefer != "" || !reflect.DeepEqual(args, in) {
			t.Errorf("got (%v, %v, %q), want (%v, true, \"\")", args, hostGiven, prefer, in)
		}
	})

	t.Run("loopback host matching no installed service passes through unchanged", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) { return "", false }
		in := []string{"-h", "127.0.0.1", "-p", "5555", "mydb"}
		args, hostGiven, prefer := resolveLoopbackTarget("psql", in)
		if !hostGiven || prefer != "" || !reflect.DeepEqual(args, in) {
			t.Errorf("got (%v, %v, %q), want (%v, true, \"\")", args, hostGiven, prefer, in)
		}
	})

	t.Run("loopback host with explicit matching port is rewritten", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			if tool == "psql" && port == "5433" {
				return "postgres-timescaledb", true
			}
			return "", false
		}
		args, hostGiven, prefer := resolveLoopbackTarget("psql",
			[]string{"-h", "127.0.0.1", "-p", "5433", "-U", "postgres", "-d", "lerd"})
		want := []string{"-U", "postgres", "-d", "lerd"}
		if hostGiven || prefer != "postgres-timescaledb" || !reflect.DeepEqual(args, want) {
			t.Errorf("got (%v, %v, %q), want (%v, false, postgres-timescaledb)", args, hostGiven, prefer, want)
		}
	})

	t.Run("loopback host with no port matches the family default port", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			if tool == "psql" && port == "5432" {
				return "postgres", true
			}
			return "", false
		}
		args, hostGiven, prefer := resolveLoopbackTarget("psql", []string{"-h", "localhost", "-U", "postgres"})
		want := []string{"-U", "postgres"}
		if hostGiven || prefer != "postgres" || !reflect.DeepEqual(args, want) {
			t.Errorf("got (%v, %v, %q), want (%v, false, postgres)", args, hostGiven, prefer, want)
		}
	})

	// A URI carries its host and port inside one token, so rewriting it would mean
	// parsing and rebuilding it. It stays external even when it spells a loopback
	// host: passing it through unchanged is what 1.31.0 did, and it keeps lerd's
	// credentials away from a connection lerd does not own.
	t.Run("a connection URI naming a loopback host is left alone", func(t *testing.T) {
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			t.Fatalf("must not be called for a connection URI")
			return "", false
		}
		in := []string{"postgresql://user:pw@127.0.0.1:5432/mydb"}
		args, hostGiven, prefer := resolveLoopbackTarget("psql", in)
		if !hostGiven || prefer != "" || !reflect.DeepEqual(args, in) {
			t.Errorf("got (%v, %v, %q), want (%v, true, \"\")", args, hostGiven, prefer, in)
		}
	})

	t.Run("mysql -p is never mistaken for a port when matching", func(t *testing.T) {
		var gotPort string
		loopbackServiceOwningPortFn = func(tool, port string) (string, bool) {
			gotPort = port
			return "mysql", true
		}
		args, hostGiven, prefer := resolveLoopbackTarget("mysql", []string{"-h", "127.0.0.1", "-psecret", "mydb"})
		if gotPort != "3306" {
			t.Errorf("port matched against = %q, want the family default 3306 (mysql's -p is --password)", gotPort)
		}
		want := []string{"-psecret", "mydb"}
		if hostGiven || prefer != "mysql" || !reflect.DeepEqual(args, want) {
			t.Errorf("got (%v, %v, %q), want (%v, false, mysql) with -psecret preserved", args, hostGiven, prefer, want)
		}
	})
}
