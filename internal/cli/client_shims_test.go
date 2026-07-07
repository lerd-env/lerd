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
