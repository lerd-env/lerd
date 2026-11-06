package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every command has to lose lerd's server, or it would ask a container with no
// server in it, and each action must survive the rewrite whole.
func TestExternalIntrospect_DropsLerdsServerFromEveryCommand(t *testing.T) {
	for _, name := range []string{"mysql", "postgres"} {
		t.Run(name, func(t *testing.T) {
			p, err := LoadPreset(name)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ExternalIntrospect(p.Introspect)
			if err != nil {
				t.Fatal(err)
			}
			src, dst := p.Introspect.DatabasesEntity(), got.DatabasesEntity()
			if dst == nil || len(dst.Actions) != len(src.Actions) {
				t.Fatalf("actions lost: %+v", dst)
			}
			if dst.Actions["drop"].Destructive != src.Actions["drop"].Destructive || dst.Actions["export"].Filename != src.Actions["export"].Filename {
				t.Errorf("action options lost: %+v", dst.Actions)
			}
			cmds := []string{dst.List}
			for _, a := range dst.Actions {
				if a.Exec == "" {
					t.Errorf("an action lost its command: %+v", dst.Actions)
				}
				cmds = append(cmds, a.Exec)
			}
			for _, cmd := range cmds {
				if !strings.HasPrefix(cmd, "unset MYSQL_PWD PGPASSWORD; ") {
					t.Errorf("lerd's own password would reach it: %s", cmd)
				}
				if strings.Contains(cmd, "127.0.0.1") || strings.Contains(cmd, "-uroot") || strings.Contains(cmd, "-U postgres") {
					t.Errorf("still names lerd's server: %s", cmd)
				}
			}
		})
	}
}

func TestExternalIntrospect_RefusesWhatItCannotRewrite(t *testing.T) {
	in := &Introspect{Entities: []EntitySpec{{Kind: "databases", List: "mysql --host=127.0.0.1 -e list"}}}
	if _, err := ExternalIntrospect(in); err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("got %v", err)
	}
	if _, err := ExternalIntrospect(nil); err == nil {
		t.Fatal("a preset with no commands has nothing to run")
	}
}

func TestExternalDBClientContent(t *testing.T) {
	e := ExternalDB{Host: "host.containers.internal", Port: 3307, User: "me"}
	if got := ExternalDBClientContent("mariadb", e, `p"w\x`); !strings.Contains(got, `password="p\"w\\x"`) || !strings.Contains(got, "port=3307") {
		t.Errorf("mysql: %q", got)
	}
	if got := ExternalDBClientContent("postgres", e, `a:b\c`); got != "127.0.0.1:3307:*:me:a\\:b\\\\c\n" {
		t.Errorf("postgres: %q", got)
	}
}

func TestExternalDBVars_GoesByWhatEachKeyIs(t *testing.T) {
	e := ExternalDB{Port: 3307, User: "me"}
	got := ExternalDBVars([]string{
		"DB_CONNECTION=mysql",
		"DB_HOST=lerd-dbngin",
		"DB_PORT=3306",
		"DB_DATABASE={{site}}",
		"DB_USERNAME=root",
		"DB_PASSWORD=lerd",
		"DATABASE_URL=mysql://root:lerd@lerd-dbngin:3306/{{site}}?serverVersion=8.4",
		"db.connection.default.host=lerd-dbngin",
		"db.connection.default.password=lerd",
	}, e, "host.containers.internal", "p@ss/w")
	want := []string{
		"DB_CONNECTION=mysql",
		"DB_HOST=host.containers.internal",
		"DB_PORT=3307",
		"DB_DATABASE={{site}}",
		"DB_USERNAME=me",
		"DB_PASSWORD=p@ss/w",
		"DATABASE_URL=mysql://me:p%40ss%2Fw@host.containers.internal:3307/{{site}}?serverVersion=8.4",
		"db.connection.default.host=host.containers.internal",
		"db.connection.default.password=p@ss/w",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %s, want %s", got[i], want[i])
		}
	}
}

func externalServiceProject(t *testing.T, env string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	svc := &CustomService{Name: "dbngin", Family: "mariadb", External: &ExternalDB{Host: "host.containers.internal", Port: 3306, User: "me"}}
	if err := SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - dbngin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A worktree of such a site is isolated through the service the site chose.
func TestWorktreeDBService_NamesTheExternalServiceTheSiteChose(t *testing.T) {
	for name, env := range map[string]string{
		"from a container": "DB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop\n",
		"from the machine": "DB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := WorktreeDBService(externalServiceProject(t, env)); got != "dbngin" {
				t.Errorf("got %q", got)
			}
		})
	}
}

// Moved by hand to another server, the choice in .lerd.yaml no longer holds, and
// cloning through it would write to the wrong one.
func TestWorktreeDBService_IgnoresAChoiceTheEnvLeft(t *testing.T) {
	for name, env := range map[string]string{
		"another host": "DB_HOST=db.example.com\nDB_PORT=3306\nDB_DATABASE=shop\n",
		"another port": "DB_HOST=host.containers.internal\nDB_PORT=3307\nDB_DATABASE=shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := WorktreeDBService(externalServiceProject(t, env)); got == "dbngin" {
				t.Errorf("got %q", got)
			}
		})
	}
}

func TestExternalDBEnvMatches(t *testing.T) {
	dir := externalServiceProject(t, "DB_HOST=host.containers.internal\nDB_PORT=3306\n")
	svc, err := LoadCustomService("dbngin")
	if err != nil {
		t.Fatal(err)
	}
	if !ExternalDBEnvMatches(dir, svc) {
		t.Error("the env names the server")
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\nDB_PORT=3306\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ExternalDBEnvMatches(dir, svc) {
		t.Error("a checkout on lerd's service does not name it")
	}
}

// WordPress has no port key, so a server off the default port has to travel in
// the host, or the site connects to the wrong place.
func TestExternalDBVars_NoPortKeyPutsThePortInTheHost(t *testing.T) {
	got := ExternalDBVars([]string{"DB_NAME=wp", "DB_USER=root", "DB_PASSWORD=lerd", "DB_HOST=lerd-mysql"}, ExternalDB{Port: 3310, User: "me"}, "host.containers.internal", "pw")
	if got[3] != "DB_HOST=host.containers.internal:3310" || got[1] != "DB_USER=me" || got[2] != "DB_PASSWORD=pw" {
		t.Errorf("got %v", got)
	}
}

func TestDBEndpointFor(t *testing.T) {
	for name, tc := range map[string]struct {
		env  string
		want DBEndpoint
	}{
		"separate keys": {"DB_HOST=db.lan\nDB_PORT=3307\nDB_DATABASE=shop\n", DBEndpoint{Host: "db.lan", Port: "3307", Database: "shop"}},
		"port in host":  {"DB_HOST=db.lan:3310\nDB_DATABASE=shop\n", DBEndpoint{Host: "db.lan", Port: "3310", Database: "shop"}},
		"nothing set":   {"APP_NAME=x\n", DBEndpoint{}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.env), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := DBEndpointFor(dir); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestEndpointFromURL(t *testing.T) {
	got := endpointFromURL(`"mysql://me:p%40ss@host.containers.internal:3310/shop?serverVersion=8.4"`)
	if got != (DBEndpoint{Host: "host.containers.internal", Port: "3310", Database: "shop"}) {
		t.Errorf("got %+v", got)
	}
}

// Owner badges and automatic snapshots both start from these targets.
func TestDBTargetsFor_NamesTheExternalServerASiteChose(t *testing.T) {
	dir := externalServiceProject(t, "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop\n")
	got := DBTargetsFor(dir)
	if len(got) != 1 || got[0].Service != "dbngin" || got[0].Database != "shop" {
		t.Fatalf("targets = %+v", got)
	}
}

func TestDBTargetsFor_IgnoresAServerTheEnvLeft(t *testing.T) {
	dir := externalServiceProject(t, "DB_CONNECTION=mysql\nDB_HOST=elsewhere.example\nDB_PORT=3306\nDB_DATABASE=shop\n")
	for _, target := range DBTargetsFor(dir) {
		if target.Service == "dbngin" {
			t.Fatalf("targets = %+v", target)
		}
	}
}

// db:use writes .lerd.yaml in the site's folder, and until that is committed a
// worktree has none, yet it is on the same server.
func TestExternalDBServiceFor_AWorktreeFollowsItsParentsChoice(t *testing.T) {
	site := externalServiceProject(t, "DB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop\n")
	checkout := filepath.Join(t.TempDir(), "shop-feature")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".env"), []byte("DB_HOST=host.containers.internal\nDB_PORT=3306\nDB_DATABASE=shop_feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeWorktreeMeta(t, site, "feature", checkout)
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: site}); err != nil {
		t.Fatal(err)
	}
	if got := ExternalDBServiceFor(checkout); got == nil || got.Name != "dbngin" {
		t.Fatalf("got %+v", got)
	}
}

func TestDBURLWithName(t *testing.T) {
	for name, tc := range map[string]struct{ raw, db, want string }{
		"keeps the query":     {"mysql://me:p%40ss@h:3307/shop?charset=utf8mb4", "shop_b", "mysql://me:p%40ss@h:3307/shop_b?charset=utf8mb4"},
		"keeps the quotes":    {`"mysql://me:pw@h/shop"`, "shop_b", `"mysql://me:pw@h/shop_b"`},
		"keeps a fragment":    {"postgresql://me@h/shop#note", "shop_b", "postgresql://me@h/shop_b#note"},
		"escapes the name":    {"postgresql://me@h/shop", "shop#feature", "postgresql://me@h/shop%23feature"},
		"no database in path": {"mysql://me@h", "shop", "mysql://me@h"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := DBURLWithName(tc.raw, tc.db); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A round trip must not change what the name means.
func TestDBNameInURL_RoundTripsThroughDBURLWithName(t *testing.T) {
	const raw = "postgresql://me@h/shop%23feature"
	if got := DBURLWithName("postgresql://me@h/other", DBNameInURL(raw)); got != raw {
		t.Errorf("got %q", got)
	}
}
