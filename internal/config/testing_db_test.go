package config

import (
	"slices"
	"testing"
)

// laravelShapedDef detects MySQL by DB_CONNECTION and names the database in
// DB_DATABASE, as the published Laravel definition does.
func laravelShapedDef() *Framework {
	return &Framework{
		Name:   "shaped",
		Detect: []FrameworkRule{{File: "shaped.lock"}},
		Env: FrameworkEnvConf{
			File: ".env",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "mysql"}},
					Vars:   []string{"DB_CONNECTION=mysql", "DB_HOST=lerd-mysql", "DB_DATABASE={{site}}"},
				},
			},
		},
	}
}

func phpunitWith(vars string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<phpunit><php>
    <ini name="DB_CONNECTION" value="ignored"/>
` + vars + `
</php></phpunit>`
}

// A site whose tests run elsewhere had an empty <db>_testing created beside its
// database every time it was wired (#2668).
func TestDatabasesToCreate(t *testing.T) {
	const env = "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n"
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"no PHPUnit config keeps the twin", nil, []string{"shop", "shop_testing"}},
		{"a config silent on the database keeps it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="APP_ENV" value="testing"/>`),
		}, []string{"shop", "shop_testing"}},
		{"commented-out SQLite lines keep it", map[string]string{
			"phpunit.xml": phpunitWith(`<!-- <env name="DB_CONNECTION" value="sqlite"/> -->`),
		}, []string{"shop", "shop_testing"}},
		{"tests on in-memory SQLite drop it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_CONNECTION" value="sqlite"/><env name="DB_DATABASE" value=":memory:"/>`),
		}, []string{"shop"}},
		{"a server var counts as well as an env one", map[string]string{
			"phpunit.xml": phpunitWith(`<server name="DB_CONNECTION" value="sqlite"/>`),
		}, []string{"shop"}},
		{"the stock empty DB_URL keeps it", map[string]string{
			"phpunit.xml": phpunitWith(`<!-- <env name="DB_CONNECTION" value="sqlite"/> --><env name="DB_URL" value=""/>`),
		}, []string{"shop", "shop_testing"}},
		{"a connection lerd can't read keeps it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_CONNECTION" value="mysql_testing"/><env name="DB_DATABASE" value="shop"/>`),
		}, []string{"shop", "shop_testing"}},
		{"tests on a URL naming another database drop it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_URL" value="mysql://root:lerd@lerd-mysql:3306/shop_test"/>`),
		}, []string{"shop"}},
		{"tests on a SQLite URL drop it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_URL" value="sqlite:///:memory:"/>`),
		}, []string{"shop"}},
		{"tests naming the twin keep it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_DATABASE" value="shop_testing"/>`),
		}, []string{"shop", "shop_testing"}},
		{"tests on another database drop it", map[string]string{
			"phpunit.xml": phpunitWith(`<env name="DB_DATABASE" value="shop_test"/>`),
		}, []string{"shop"}},
		{"the committed dist file is read when there is no local one", map[string]string{
			"phpunit.xml.dist": phpunitWith(`<env name="DB_CONNECTION" value="sqlite"/>`),
		}, []string{"shop"}},
		{"a local phpunit.xml wins over the dist file", map[string]string{
			"phpunit.xml":      phpunitWith(`<env name="APP_ENV" value="testing"/>`),
			"phpunit.xml.dist": phpunitWith(`<env name="DB_CONNECTION" value="sqlite"/>`),
		}, []string{"shop", "shop_testing"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setConfigDir(t)
			installStoreFramework(t, laravelShapedDef())
			dir := t.TempDir()
			writeProjectFile(t, dir, "shaped.lock", "")
			writeProjectFile(t, dir, ".env", env)
			for name, body := range c.files {
				writeProjectFile(t, dir, name, body)
			}
			if got := DatabasesToCreate(dir, "shop", nil); !slices.Equal(got, c.want) {
				t.Errorf("DatabasesToCreate = %v, want %v", got, c.want)
			}
		})
	}
}

// lerd env decides before it writes, so a project still on SQLite in its .env
// that is being moved to MySQL is judged on the env it is about to have.
func TestDatabasesToCreateReadsTheEnvAboutToBeWritten(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, laravelShapedDef())
	dir := t.TempDir()
	writeProjectFile(t, dir, "shaped.lock", "")
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=sqlite\n")
	writeProjectFile(t, dir, "phpunit.xml", phpunitWith(`<env name="DB_DATABASE" value="shop_test"/>`))

	next := map[string]string{"DB_CONNECTION": "mysql", "DB_HOST": "lerd-mysql", "DB_DATABASE": "shop"}
	if got := DatabasesToCreate(dir, "shop", next); !slices.Equal(got, []string{"shop"}) {
		t.Errorf("DatabasesToCreate = %v, want only the site's own", got)
	}
}

// A connection of the project's own, named in its env rather than in
// phpunit.xml, may append the suffix itself, so a name alone can't rule it out.
func TestDatabasesToCreateKeepsTheTwinBehindAConnectionOfTheProjectsOwn(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, laravelShapedDef())
	dir := t.TempDir()
	writeProjectFile(t, dir, "shaped.lock", "")
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=mysql_testing\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n")
	writeProjectFile(t, dir, "phpunit.xml", phpunitWith(`<env name="DB_DATABASE" value="shop"/>`))
	if got := DatabasesToCreate(dir, "shop", nil); !slices.Equal(got, []string{"shop", "shop_testing"}) {
		t.Errorf("DatabasesToCreate = %v, want the twin kept", got)
	}
}

// The keys come from the framework definition, not from Laravel's names: a
// framework addressing its database by a DSN is read through that key.
func TestDatabasesToCreateFollowsTheDefinitionsKeys(t *testing.T) {
	cases := []struct {
		name, testURL string
		want          []string
	}{
		{"tests on SQLite", "sqlite:///:memory:", []string{"shop"}},
		{"tests on the twin", "mysql://root:lerd@lerd-mysql:3306/shop_testing", []string{"shop", "shop_testing"}},
		{"tests on the twin over loopback", "mysql://root:lerd@127.0.0.1:3306/shop_testing", []string{"shop", "shop_testing"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setConfigDir(t)
			installStoreFramework(t, &Framework{
				Name:   "acme",
				Detect: []FrameworkRule{{File: "acme.lock"}},
				Env: FrameworkEnvConf{
					File: ".env",
					Services: map[string]FrameworkServiceDef{
						"mysql": {
							Detect: []FrameworkServiceDetect{{Key: "DATABASE_URL"}},
							Vars:   []string{"DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/{{site}}"},
						},
					},
				},
			})
			dir := t.TempDir()
			writeProjectFile(t, dir, "acme.lock", "")
			writeProjectFile(t, dir, ".env", "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n")
			writeProjectFile(t, dir, "phpunit.xml.dist", phpunitWith(`<env name="DATABASE_URL" value="`+c.testURL+`"/>`))
			if got := DatabasesToCreate(dir, "shop", nil); !slices.Equal(got, c.want) {
				t.Errorf("DatabasesToCreate = %v, want %v", got, c.want)
			}
		})
	}
}

// Under the native runtime lerd env points a site at its engine on loopback, so
// a twin named in phpunit.xml must count whatever host the env carries.
func TestDatabasesToCreateKeepsATwinReachedOverLoopback(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, laravelShapedDef())
	dir := t.TempDir()
	writeProjectFile(t, dir, "shaped.lock", "")
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_DATABASE=shop\n")
	writeProjectFile(t, dir, "phpunit.xml", phpunitWith(`<env name="DB_DATABASE" value="shop_testing"/>`))
	if got := DatabasesToCreate(dir, "shop", nil); !slices.Equal(got, []string{"shop", "shop_testing"}) {
		t.Errorf("DatabasesToCreate = %v, want the twin the tests name", got)
	}
}

// A definition that detects its service by a key of its own, with no vars to
// say how it is addressed, still names the twin through that key's URL.
func TestDatabasesToCreateReadsAConnectionStringUnderAnyKey(t *testing.T) {
	cases := []struct {
		name, testDSN string
		want          []string
	}{
		{"tests on the twin", "mysql://root:lerd@lerd-mysql:3306/shop_testing", []string{"shop", "shop_testing"}},
		{"tests on the twin over loopback", "mysql://root:lerd@127.0.0.1:3306/shop_testing", []string{"shop", "shop_testing"}},
		{"tests on the twin through a PDO DSN", "mysql:host=127.0.0.1;port=3306;dbname=shop_testing", []string{"shop", "shop_testing"}},
		{"tests on SQLite", "sqlite:///:memory:", []string{"shop"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setConfigDir(t)
			installStoreFramework(t, &Framework{
				Name:   "dsnonly",
				Detect: []FrameworkRule{{File: "dsnonly.lock"}},
				Env: FrameworkEnvConf{
					File: ".env",
					Services: map[string]FrameworkServiceDef{
						"mysql": {Detect: []FrameworkServiceDetect{{Key: "APP_DSN"}}},
					},
				},
			})
			dir := t.TempDir()
			writeProjectFile(t, dir, "dsnonly.lock", "")
			// An app URL whose path happens to read like the twin is not a database.
			writeProjectFile(t, dir, ".env", "APP_DSN=mysql://root:lerd@lerd-mysql:3306/shop\nAPP_URL=https://example.test/shop_testing\n")
			writeProjectFile(t, dir, "phpunit.xml", phpunitWith(`<env name="APP_DSN" value="`+c.testDSN+`"/>`))
			if got := DatabasesToCreate(dir, "shop", nil); !slices.Equal(got, c.want) {
				t.Errorf("DatabasesToCreate = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsDBConnectionString(t *testing.T) {
	cases := map[string]bool{
		"mysql://root:lerd@127.0.0.1:3306/shop": true,
		"postgresql://root@lerd-postgres/shop":  true,
		"pdo-mysql://root@lerd-mysql/shop":      true,
		"mongodb+srv://lerd-mongo/shop":         true,
		"mysql:host=127.0.0.1;dbname=shop":      true,
		"https://example.test/shop_testing":     false,
		"redis://lerd-redis:6379/0":             false,
		"sqlite:///:memory:":                    false,
		"mysql":                                 false,
		"smtp://lerd-mailpit:1025":              false,
	}
	for v, want := range cases {
		if got := isDBConnectionString(v); got != want {
			t.Errorf("isDBConnectionString(%q) = %v, want %v", v, got, want)
		}
	}
}
