package cli

import "testing"

func TestArgsSpecifyHost(t *testing.T) {
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
		if got := argsSpecifyHost(c.args); got != c.want {
			t.Errorf("argsSpecifyHost(%v) = %v, want %v", c.args, got, c.want)
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
