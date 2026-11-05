package config

import (
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
	if got := ExternalDBClientContent("postgres", e, `a:b\c`); got != "host.containers.internal:3307:*:me:a\\:b\\\\c\n" {
		t.Errorf("postgres: %q", got)
	}
}

func TestExternalDBEnv(t *testing.T) {
	env := ExternalDBEnv("postgres", ExternalDB{Host: "h", Port: 5433, User: "me"})
	if env["PGDATABASE"] != "postgres" || env["PGHOST"] != "h" || env["PGPORT"] != "5433" || env["PGUSER"] != "me" {
		t.Errorf("env = %v", env)
	}
	if env := ExternalDBEnv("mysql", ExternalDB{}); env != nil {
		t.Errorf("mysql clients read the client file: %v", env)
	}
}
