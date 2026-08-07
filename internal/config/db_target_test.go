package config

import (
	"os"
	"path/filepath"
	"testing"
)

// wordpressStoreDef mirrors the published wordpress definition: the
// configuration lives in wp-config.php as PHP constants, the database is named
// by DB_NAME, and the host key doubles as the detection rule.
func wordpressStoreDef() *Framework {
	return &Framework{
		Name:      "wordpress",
		Label:     "WordPress",
		PublicDir: ".",
		Detect:    []FrameworkRule{{File: "wp-login.php"}},
		Env: FrameworkEnvConf{
			FallbackFile:   "wp-config.php",
			FallbackFormat: "php-const",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "DB_HOST"}},
					Vars:   []string{"DB_NAME={{site}}", "DB_HOST=lerd-mysql"},
				},
			},
		},
	}
}

func installStoreFramework(t *testing.T, fw *Framework) {
	t.Helper()
	if err := SaveStoreFramework(fw); err != nil {
		t.Fatalf("SaveStoreFramework(%s): %v", fw.Name, err)
	}
}

func writeProjectFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// A WordPress site keeps its configuration in wp-config.php and names its
// database DB_NAME. Both facts are published in the definition, so the target
// has to come out of the declaration rather than a Laravel-shaped .env read.
func TestDBTargetFor_WordPressReadsItsOwnConfigFile(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\ndefine( 'DB_NAME', 'astrolov' );\ndefine( 'DB_HOST', 'lerd-mysql' );\n")

	got, ok := DBTargetFor(dir)
	if !ok {
		t.Fatal("DBTargetFor found no database for a wired WordPress site")
	}
	if got.Service != "mysql" || got.Database != "astrolov" {
		t.Errorf("DBTargetFor = %+v, want {mysql astrolov}", got)
	}
}

// A definition declaring several database services must resolve the one its
// detect rules match, not the first one that happens to be declared.
func TestDBTargetFor_PicksTheDetectedService(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, &Framework{
		Name:   "acme",
		Detect: []FrameworkRule{{File: "acme.lock"}},
		Env: FrameworkEnvConf{
			File: ".env",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "mysql"}},
					Vars:   []string{"DB_HOST=lerd-mysql", "DB_DATABASE={{site}}"},
				},
				"postgres": {
					Detect: []FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "pgsql"}},
					Vars:   []string{"DB_HOST=lerd-postgres", "DB_DATABASE={{site}}"},
				},
			},
		},
	})

	dir := t.TempDir()
	writeProjectFile(t, dir, "acme.lock", "")
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=pgsql\nDB_HOST=lerd-postgres\nDB_DATABASE=shop\n")

	got, ok := DBTargetFor(dir)
	if !ok || got.Service != "postgres" || got.Database != "shop" {
		t.Errorf("DBTargetFor = %+v (ok=%v), want {postgres shop}", got, ok)
	}
}

// The host key names the exact engine, so an alternate build resolves to itself
// rather than collapsing to the family's canonical service.
func TestDBTargetFor_AlternateEngineKeepsItsOwnName(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\ndefine( 'DB_NAME', 'blog' );\ndefine( 'DB_HOST', 'lerd-mariadb-11-8' );\n")

	got, _ := DBTargetFor(dir)
	if got.Service != "mariadb-11-8" {
		t.Errorf("service = %q, want mariadb-11-8", got.Service)
	}
}

// A framework wiring its database through a single DSN carries the host and the
// name in one value; both come from parsing it.
func TestDBTargetFor_ResolvesADSNKey(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, &Framework{
		Name:   "acme",
		Detect: []FrameworkRule{{File: "acme.lock"}},
		Env: FrameworkEnvConf{
			File: ".env.local",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "DATABASE_URL", ValuePrefix: "mysql"}},
					Vars:   []string{"DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/{{site}}?serverVersion=8.0"},
				},
			},
		},
	})

	dir := t.TempDir()
	writeProjectFile(t, dir, "acme.lock", "")
	writeProjectFile(t, dir, ".env.local", "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n")

	got, ok := DBTargetFor(dir)
	if !ok || got.Service != "mysql" || got.Database != "shop" {
		t.Errorf("DBTargetFor = %+v (ok=%v), want {mysql shop}", got, ok)
	}
}

// A declaration that detects a service without publishing its vars still has to
// resolve: the value is read from the file the definition points at, the way
// `lerd db:shell` has always found Symfony's DATABASE_URL in .env.local.
func TestDBTargetFor_DetectedServiceWithoutVars(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, &Framework{
		Name:   "acme",
		Detect: []FrameworkRule{{File: "acme.lock"}},
		Env: FrameworkEnvConf{
			File: ".env.local",
			Services: map[string]FrameworkServiceDef{
				"mysql": {Detect: []FrameworkServiceDetect{{Key: "DATABASE_URL", ValuePrefix: "mysql"}}},
			},
		},
	})

	dir := t.TempDir()
	writeProjectFile(t, dir, "acme.lock", "")
	writeProjectFile(t, dir, ".env", "APP_ENV=dev\n")
	writeProjectFile(t, dir, ".env.local", "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n")

	got, ok := DBTargetFor(dir)
	if !ok || got.Service != "mysql" || got.Database != "shop" {
		t.Errorf("DBTargetFor = %+v (ok=%v), want {mysql shop}", got, ok)
	}
}

// A host-proxy site rewrites its host key to loopback, so the engine is
// recovered from the service the project lists in .lerd.yaml.
func TestDBTargetFor_HostProxySiteResolvesThroughLerdYAML(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\nservices:\n  - mysql\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\ndefine( 'DB_NAME', 'blog' );\ndefine( 'DB_HOST', '127.0.0.1:3306' );\n")

	got, ok := DBTargetFor(dir)
	if !ok || got.Service != "mysql" || got.Database != "blog" {
		t.Errorf("DBTargetFor = %+v (ok=%v), want {mysql blog}", got, ok)
	}
}

// A database on a host lerd doesn't manage belongs to nobody here.
func TestDBTargetFor_ExternalHostIsNotOurs(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\ndefine( 'DB_NAME', 'blog' );\ndefine( 'DB_HOST', 'db.example.com' );\n")

	if got, ok := DBTargetFor(dir); ok {
		t.Errorf("DBTargetFor = %+v, want no target for an external host", got)
	}
}

// Nothing detected means nothing declared it, and the caller has to be free to
// fall through to its own inference.
func TestDBTargetFor_NoFrameworkDeclaration(t *testing.T) {
	setConfigDir(t)

	dir := t.TempDir()
	writeProjectFile(t, dir, ".env", "DB_HOST=lerd-mysql\nDB_DATABASE=shop\n")

	if got, ok := DBTargetFor(dir); ok {
		t.Errorf("DBTargetFor = %+v, want no declared target without a framework", got)
	}
}

// The card asks a different question than the CLI: any lerd-managed database
// the project points at, declared or not, so an unrecognised project still
// shows up as the owner of its database.
func TestDBTargetsFor_UndeclaredProjectStillResolves(t *testing.T) {
	setConfigDir(t)

	dir := t.TempDir()
	writeProjectFile(t, dir, ".env", "DB_HOST=lerd-mysql\nDB_DATABASE=shop\n")

	got := DBTargetsFor(dir)
	if len(got) != 1 || got[0].Service != "mysql" || got[0].Database != "shop" {
		t.Errorf("DBTargetsFor = %+v, want [{mysql shop}]", got)
	}
}

// An engine wired through a connection string alone (mongo) carries no host
// key, so it is found by walking the values for a DSN aimed at a lerd engine.
func TestDBTargetsFor_FindsADatabaseBehindADSN(t *testing.T) {
	setConfigDir(t)

	dir := t.TempDir()
	writeProjectFile(t, dir, ".env", "DB_HOST=lerd-mysql\nDB_DATABASE=shop\nQUEUE_DSN=mongodb://root:lerd@lerd-mongo:27017/jobs?authSource=admin\nCACHE_URL=redis://lerd-redis:6379/0\n")

	got := DBTargetsFor(dir)
	if len(got) != 2 {
		t.Fatalf("DBTargetsFor = %+v, want the mysql and mongo databases", got)
	}
	if got[1].Service != "mongo" || got[1].Database != "jobs" {
		t.Errorf("second target = %+v, want {mongo jobs}", got[1])
	}
}

// Go randomises map iteration, so a project carrying more than one DSN against
// the same engine would be attributed to a different database on every
// snapshot. The walk has to be ordered.
func TestDBTargetsFor_DSNWalkIsStable(t *testing.T) {
	setConfigDir(t)

	dir := t.TempDir()
	writeProjectFile(t, dir, ".env", "QUEUE_DSN=mongodb://root:lerd@lerd-mongo:27017/jobs\nANALYTICS_DSN=mongodb://root:lerd@lerd-mongo:27017/metrics\n")

	for range 50 {
		got := DBTargetsFor(dir)
		// The alphabetically first key wins, so the answer is predictable
		// rather than merely repeatable.
		if len(got) != 1 || got[0].Database != "metrics" {
			t.Fatalf("DBTargetsFor = %+v, want the single stable {mongo metrics}", got)
		}
	}
}

// A site wired through a single connection string keeps its engine and its
// database in the one value, and everything else in the file has to be left
// alone.
func TestDSNTarget(t *testing.T) {
	setConfigDir(t)
	cases := []struct {
		value string
		want  DBTarget
	}{
		{"mongodb://root:lerd@lerd-mongo:27017/astrolov?authSource=admin", DBTarget{"mongo", "astrolov"}},
		{"mysql://root:lerd@lerd-mysql:3306/shop", DBTarget{"mysql", "shop"}},
		// An alternate instance is its own engine, not the canonical one.
		{"mongodb://root:lerd@lerd-mongo-6:27017/astrolov", DBTarget{"mongo-6", "astrolov"}},
		{"mongodb://root:lerd@lerd-mongo:27017/?authSource=admin", DBTarget{}},
		{"not a url at all", DBTarget{}},
		// A cache's trailing digit is an index, not a database name.
		{"redis://lerd-redis:6379/0", DBTarget{}},
		{"redis://lerd-redis:6379", DBTarget{}},
		// A multi-segment path is an object key, not a database name.
		{"http://lerd-rustfs:9000/bucket/key", DBTarget{}},
		// A database on somebody else's server is not ours to claim.
		{"mysql://root:lerd@db.example.com:3306/shop", DBTarget{}},
	}
	for _, tc := range cases {
		if got := dsnTarget(tc.value); got != tc.want {
			t.Errorf("dsnTarget(%q) = %+v, want %+v", tc.value, got, tc.want)
		}
	}
}

// The writer side needs a file, a format and a pair of keys to aim at even when
// nothing is wired yet, so it falls back to the shape Laravel gave everyone.
func TestDBEnvBindingFor_FallsBackToTheDefaultShape(t *testing.T) {
	setConfigDir(t)

	dir := t.TempDir()
	got := DBEnvBindingFor(dir)
	want := DBEnvBinding{File: ".env", Format: "dotenv", HostKey: "DB_HOST", NameKey: "DB_DATABASE"}
	if got != want {
		t.Errorf("DBEnvBindingFor = %+v, want %+v", got, want)
	}
}

// The binding is what db:isolate writes through, so it has to address each
// framework's database the way that framework actually stores it.
func TestDBEnvBindingFor_MagentoStyleDottedKeys(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, &Framework{
		Name: "magish",
		Env: FrameworkEnvConf{
			File:   "app/etc/env.php",
			Format: "php-array",
			URLKey: "none",
			Services: map[string]FrameworkServiceDef{
				"mysql": {Vars: []string{
					"db.connection.default.host=lerd-mysql",
					"db.connection.default.dbname={{site}}",
				}},
			},
		},
	})

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: magish\n")

	got := DBEnvBindingFor(dir)
	want := DBEnvBinding{
		File:    "app/etc/env.php",
		Format:  "php-array",
		HostKey: "db.connection.default.host",
		NameKey: "db.connection.default.dbname",
	}
	if got != want {
		t.Errorf("DBEnvBindingFor = %+v, want %+v", got, want)
	}
}

func TestDBEnvBindingFor_WordPressKeysAndFormat(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\ndefine( 'DB_NAME', 'blog' );\ndefine( 'DB_HOST', 'lerd-mysql' );\n")

	got := DBEnvBindingFor(dir)
	want := DBEnvBinding{File: "wp-config.php", Format: "php-const", HostKey: "DB_HOST", NameKey: "DB_NAME"}
	if got != want {
		t.Errorf("DBEnvBindingFor = %+v, want %+v", got, want)
	}
}

// A DSN-only framework has no standalone name key to rewrite, so the writer
// gets the default shape and reports the site as unmanaged, as it always has.
func TestDBEnvBindingFor_DSNFrameworkKeepsTheDefaultShape(t *testing.T) {
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

	got := DBEnvBindingFor(dir)
	if got.NameKey != "DB_DATABASE" || got.HostKey != "DB_HOST" {
		t.Errorf("DBEnvBindingFor = %+v, want the default DB_HOST/DB_DATABASE shape", got)
	}
}

func TestDBServiceFor(t *testing.T) {
	setConfigDir(t)

	// A container or PHP site names the service directly in its host key.
	if got := DBServiceFor("", "lerd-postgres"); got != "postgres" {
		t.Errorf("lerd-postgres -> %q, want postgres", got)
	}
	if got := DBServiceFor("", "lerd-mariadb-11"); got != "mariadb-11" {
		t.Errorf("lerd-mariadb-11 -> %q, want mariadb-11", got)
	}

	// A host-proxy site rewrites the host to loopback, so the service comes
	// from the .lerd.yaml services list (postgres here, redis ignored).
	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "services:\n  - redis\n  - postgres\nproxy:\n  command: npm run start:dev\n  port: 3100\n")
	if got := DBServiceFor(dir, "127.0.0.1"); got != "postgres" {
		t.Errorf("host-proxy loopback -> %q, want postgres (from .lerd.yaml)", got)
	}

	noDB := t.TempDir()
	writeProjectFile(t, noDB, ".lerd.yaml", "services:\n  - redis\n")
	if got := DBServiceFor(noDB, "127.0.0.1"); got != "" {
		t.Errorf("no db service -> %q, want empty", got)
	}
}

// The env file a project keeps its configuration in is the framework's to
// declare, and the fallback only applies when the primary isn't there.
func TestEnvFileFor(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, wordpressStoreDef())

	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "framework: wordpress\n")
	writeProjectFile(t, dir, "wp-config.php", "<?php\n")

	file, format := EnvFileFor(dir)
	if file != "wp-config.php" || format != "php-const" {
		t.Errorf("EnvFileFor = (%q, %q), want (wp-config.php, php-const)", file, format)
	}

	bare := t.TempDir()
	if file, format := EnvFileFor(bare); file != ".env" || format != "dotenv" {
		t.Errorf("EnvFileFor(bare) = (%q, %q), want (.env, dotenv)", file, format)
	}
}
