package config

import "testing"

// acmeWithSQLite is a framework that wires both a database service and a
// SQLite file, the way the published Laravel definition does.
func acmeWithSQLite() *Framework {
	return &Framework{
		Name:   "acme",
		Detect: []FrameworkRule{{File: "acme.lock"}},
		Env: FrameworkEnvConf{
			File: ".env",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "mysql"}},
					Vars:   []string{"DB_HOST=lerd-mysql", "DB_DATABASE={{site}}"},
				},
			},
			SQLite: &FrameworkServiceDef{
				Detect: []FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "sqlite"}},
			},
		},
	}
}

func TestHasDatabase(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, acmeWithSQLite())

	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a database service the framework detects", map[string]string{"acme.lock": "", ".env": "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n"}, true},
		// The framework's default file is used when the env names none.
		{"a SQLite connection without a file named", map[string]string{"acme.lock": "", ".env": "DB_CONNECTION=sqlite\n"}, true},
		{"a db block in .lerd.yaml", map[string]string{".lerd.yaml": "db:\n  service: postgres\n  database: shop\n"}, true},
		{"an env with no database at all", map[string]string{"acme.lock": "", ".env": "APP_NAME=shop\n"}, false},
		{"no framework and no env", map[string]string{"index.html": ""}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				writeProjectFile(t, dir, name, body)
			}
			if got := HasDatabase(dir); got != tc.want {
				t.Errorf("HasDatabase = %v, want %v", got, tc.want)
			}
		})
	}
}
