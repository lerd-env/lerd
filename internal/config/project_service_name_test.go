package config

import "testing"

func TestProjectServiceFromName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	inline := &CustomService{Name: "search-2"}
	for _, c := range []struct {
		in              ProjectService
		preset, version string
	}{
		{ProjectService{Name: "mariadb-12-3"}, "mariadb", "12.3"},
		{ProjectService{Name: "mariadb-11"}, "mariadb", "11"},
		{ProjectService{Name: "mysql-8-0-34"}, "mysql", "8.0.34"},
		{ProjectService{Name: "postgres-pgvector-19"}, "postgres-pgvector", "19"},
		// The mistake the dashboard used to write.
		{ProjectService{Name: "mariadb-12-3", Preset: "mariadb-12-3"}, "mariadb", "12.3"},
		// What the file spells out wins over the name.
		{ProjectService{Name: "mariadb-12-3", Preset: "mariadb", PresetVersion: "11.8"}, "mariadb", "11.8"},
		{ProjectService{Name: "mariadb-12-3", Preset: "mariadb-12-3", PresetVersion: "12.3"}, "mariadb-12-3", "12.3"},
		{ProjectService{Name: "redis"}, "", ""},
		{ProjectService{Name: "office-db"}, "", ""},
		{ProjectService{Name: "s3"}, "", ""},
		{ProjectService{Name: "12-3"}, "", ""},
		{ProjectService{Name: "pg-17beta"}, "", ""},
		{ProjectService{Name: "search-2", Custom: inline}, "", ""},
	} {
		got := c.in.FromName()
		if got.Name != c.in.Name || got.Preset != c.preset || got.PresetVersion != c.version || got.Custom != c.in.Custom {
			t.Errorf("%+v: got %+v", c.in, got)
		}
	}
}

// mysql-8-4 is the canonical version, whose service is plain mysql: resolving
// the repeated name to it would write another service's unit.
func TestProjectServiceResolveTakesARepeatedNameOnlyWhenThePresetProducesIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	svc, err := ProjectService{Name: "mysql-5-7", Preset: "mysql-5-7"}.Resolve()
	if err != nil || svc.Name != "mysql-5-7" || svc.PresetVersion != "5.7" {
		t.Errorf("mysql-5-7: %+v, %v", svc, err)
	}
	for _, name := range []string{"mysql-8-4", "mysql-1-1", "nope-1"} {
		if svc, err := (ProjectService{Name: name, Preset: name}).Resolve(); err == nil {
			t.Errorf("%s resolved to %s", name, svc.Name)
		}
	}
}
