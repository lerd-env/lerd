package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// A DSN-only framework has no standalone name key, so the binding names the key
// holding the whole connection and writers rename the database inside it.
func TestDBEnvBindingFor_DSNFrameworkNamesItsURLKey(t *testing.T) {
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
	if got.URLKey != "DATABASE_URL" || got.NameKey != "" || got.HostKey != "" {
		t.Errorf("DBEnvBindingFor = %+v, want the URL key and no separate keys", got)
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

// A database on someone else's server is not attributed to a lerd engine just
// because the project's .lerd.yaml happens to list one. The .lerd.yaml fallback
// exists for the host-proxy rewrite, where the host is loopback.
func TestDBServiceFor_ExternalHostIsNobodys(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "services:\n  - mysql\n")

	for _, host := range []string{"db.internal.example.com", "10.0.0.7", "db.example.com:3306"} {
		if got := DBServiceFor(dir, host); got != "" {
			t.Errorf("DBServiceFor(%q) = %q, want empty: that database is not ours", host, got)
		}
	}
	// Loopback in its several spellings is the host-proxy rewrite, which does
	// resolve through .lerd.yaml.
	for _, host := range []string{"127.0.0.1", "127.0.0.1:3306", "localhost", "::1", "[::1]:3306"} {
		if got := DBServiceFor(dir, host); got != "mysql" {
			t.Errorf("DBServiceFor(%q) = %q, want mysql", host, got)
		}
	}
}

// A project moved to SQLite keeps DB_HOST as a leftover, because writing env
// vars sets keys and never removes them. Reading the default pair anyway turns
// a file path into a schema the doctor then reports missing from an engine.
func TestDBTargetsFor_SQLiteProjectHasNoServerTarget(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	writeProjectFile(t, dir, ".lerd.yaml", "services:\n  - mysql\n")
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=sqlite\nDB_HOST=lerd-mysql\nDB_DATABASE=database/database.sqlite\n")

	if got := DBTargetsFor(dir); len(got) != 0 {
		t.Errorf("DBTargetsFor = %+v, want none for a project on SQLite", got)
	}

	// The same project on a server driver still resolves.
	writeProjectFile(t, dir, ".env", "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_DATABASE=shop\n")
	got := DBTargetsFor(dir)
	if len(got) != 1 || got[0].Service != "mysql" || got[0].Database != "shop" {
		t.Errorf("DBTargetsFor = %+v, want {mysql shop}", got)
	}
}

// The services snapshot asks for every site's env file once per service, and
// an uncached answer reads every framework definition from disk each time.
func TestFrameworkFor_ReusesAnswerWithinTTL(t *testing.T) {
	dir := t.TempDir()
	first := frameworkFor(dir)
	frameworkForCache.Lock()
	e, ok := frameworkForCache.entries[dir]
	frameworkForCache.Unlock()
	if !ok || e.fw != first {
		t.Fatal("first answer was not cached")
	}
	frameworkForCache.Lock()
	frameworkForCache.entries[dir] = frameworkForEntry{fw: &Framework{Name: "sentinel"}, at: time.Now()}
	frameworkForCache.Unlock()
	if got := frameworkFor(dir); got == nil || got.Name != "sentinel" {
		t.Errorf("fresh entry not reused, got %v", got)
	}
	frameworkForCache.Lock()
	frameworkForCache.entries[dir] = frameworkForEntry{fw: &Framework{Name: "sentinel"}, at: time.Now().Add(-frameworkForTTL)}
	frameworkForCache.Unlock()
	if got := frameworkFor(dir); got != nil && got.Name == "sentinel" {
		t.Error("expired entry was reused")
	}
}

// A long-running dashboard visits many worktrees; expired entries for deleted
// ones must not stay in memory for the life of the process.
func TestFrameworkFor_DropsExpiredEntriesOnStore(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "deleted-worktree")
	frameworkForCache.Lock()
	if frameworkForCache.entries == nil {
		frameworkForCache.entries = map[string]frameworkForEntry{}
	}
	frameworkForCache.entries[gone] = frameworkForEntry{at: time.Now().Add(-frameworkForTTL)}
	frameworkForCache.Unlock()

	frameworkFor(t.TempDir())

	frameworkForCache.Lock()
	_, ok := frameworkForCache.entries[gone]
	frameworkForCache.Unlock()
	if ok {
		t.Error("expired entry survived a store")
	}
}

// worktreeDBProject is a project listing mysql in .lerd.yaml with the env given.
func worktreeDBProject(t *testing.T, env, override string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if override != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env.lerd_override"), []byte(override), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func publishMySQL(t *testing.T, port int) {
	t.Helper()
	if err := UpdateGlobal(func(c *GlobalConfig) {
		sc := c.Services["mysql"]
		sc.Port = port
		if c.Services == nil {
			c.Services = map[string]ServiceConfig{}
		}
		c.Services["mysql"] = sc
	}); err != nil {
		t.Fatal(err)
	}
}

// A host-proxy site on lerd's MySQL reads loopback on lerd's published port.
func TestWorktreeDBService_LoopbackOnLerdsPortIsLerds(t *testing.T) {
	dir := worktreeDBProject(t, "DB_HOST=127.0.0.1\nDB_PORT=3308\nDB_DATABASE=acme\n", "")
	publishMySQL(t, 3308)
	if got := WorktreeDBService(dir); got != "mysql" {
		t.Errorf("got %q", got)
	}
}

// Loopback on any other port is a server on the machine, and treating it as
// lerd's would clone into, or drop from, the wrong one.
func TestWorktreeDBService_LoopbackOnAnotherPortIsNotLerds(t *testing.T) {
	dir := worktreeDBProject(t, "DB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=acme\n", "")
	publishMySQL(t, 3308)
	if got := WorktreeDBService(dir); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestWorktreeDBService_MarkedExternalIsNotLerds(t *testing.T) {
	dir := worktreeDBProject(t, "DB_HOST=127.0.0.1\nDB_PORT=3308\nDB_DATABASE=acme\n", "LERD_EXTERNAL_SERVICES=mysql\n")
	publishMySQL(t, 3308)
	if got := WorktreeDBService(dir); got != "" {
		t.Errorf("got %q", got)
	}
}

// With no port to compare, the site keeps working as it did.
func TestWorktreeDBService_LoopbackWithoutAPortStaysLerds(t *testing.T) {
	dir := worktreeDBProject(t, "DB_HOST=127.0.0.1\nDB_DATABASE=acme\n", "")
	publishMySQL(t, 3308)
	if got := WorktreeDBService(dir); got != "mysql" {
		t.Errorf("got %q", got)
	}
}

func TestWorktreeDBService_ContainerHostIsLerdsWhateverThePort(t *testing.T) {
	dir := worktreeDBProject(t, "DB_HOST=lerd-mysql\nDB_PORT=3306\nDB_DATABASE=acme\n", "")
	publishMySQL(t, 3308)
	if got := WorktreeDBService(dir); got != "mysql" {
		t.Errorf("got %q", got)
	}
}

// Every shape a framework declares its database in has to read and rewrite the
// same way, whichever one the store definition uses.
func TestDBEnvBinding_ReadsAndRenamesEveryShape(t *testing.T) {
	for name, tc := range map[string]struct {
		binding DBEnvBinding
		env     map[string]string
		wantDB  string
		want    map[string]string
	}{
		"separate keys": {
			DBEnvBinding{HostKey: "DB_HOST", NameKey: "DB_DATABASE"},
			map[string]string{"DB_DATABASE": "shop"}, "shop",
			map[string]string{"DB_DATABASE": "shop_b"},
		},
		"dotted keys": {
			DBEnvBinding{HostKey: "db.connection.default.host", NameKey: "db.connection.default.dbname"},
			map[string]string{"db.connection.default.dbname": "shop"}, "shop",
			map[string]string{"db.connection.default.dbname": "shop_b"},
		},
		"connection url": {
			DBEnvBinding{URLKey: "DATABASE_URL"},
			map[string]string{"DATABASE_URL": "mysql://root:lerd@lerd-mysql:3306/shop?serverVersion=8.4"}, "shop",
			map[string]string{"DATABASE_URL": "mysql://root:lerd@lerd-mysql:3306/shop_b?serverVersion=8.4"},
		},
		"pdo dsn": {
			DBEnvBinding{URLKey: "dsn"},
			map[string]string{"dsn": "mysql:host=lerd-mysql;dbname=shop"}, "shop",
			map[string]string{"dsn": "mysql:host=lerd-mysql;dbname=shop_b"},
		},
		"pdo dsn with more after the name": {
			DBEnvBinding{URLKey: "dsn"},
			map[string]string{"dsn": "pgsql:host=lerd-postgres;dbname=shop;port=5432"}, "shop",
			map[string]string{"dsn": "pgsql:host=lerd-postgres;dbname=shop_b;port=5432"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			read := func(k string) string { return tc.env[k] }
			if got := tc.binding.Database(read); got != tc.wantDB {
				t.Errorf("database = %q, want %q", got, tc.wantDB)
			}
			got := tc.binding.NameUpdate(read, "shop_b")
			if len(got) != len(tc.want) {
				t.Fatalf("update = %v", got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestDSNTarget_ReadsAPDODSN(t *testing.T) {
	for value, want := range map[string]DBTarget{
		"mysql:host=lerd-mysql;dbname=shop":           {Service: "mysql", Database: "shop"},
		"pgsql:host=lerd-postgres;port=5432;dbname=x": {Service: "postgres", Database: "x"},
		"mysql:host=127.0.0.1;dbname=shop":            {},
		"mysql:host=lerd-mysql":                       {},
	} {
		if got := dsnTarget(value); got != want {
			t.Errorf("dsnTarget(%q) = %+v, want %+v", value, got, want)
		}
	}
}

// A value the file computes, such as getenv('DB_USER'), cannot be read back, and
// copying it as empty would wipe the expression out of the worktree.
func TestDBURLConnection_LeavesOutWhatItCannotRead(t *testing.T) {
	setConfigDir(t)
	installStoreFramework(t, &Framework{
		Name:   "acme",
		Detect: []FrameworkRule{{File: "acme.lock"}},
		Env: FrameworkEnvConf{
			File:   "db.php",
			Format: "php-array",
			Services: map[string]FrameworkServiceDef{
				"mysql": {
					Detect: []FrameworkServiceDetect{{Key: "dsn", ValuePrefix: "mysql:"}},
					Vars:   []string{"dsn=mysql:host=lerd-mysql;dbname={{site}}", "username=root", "password=lerd"},
				},
			},
		},
	})
	dir := t.TempDir()
	writeProjectFile(t, dir, "acme.lock", "")
	writeProjectFile(t, dir, "db.php", "<?php\nreturn ['dsn' => 'mysql:host=lerd-mysql;dbname=shop', 'username' => getenv('DB_USER'), 'password' => ''];\n")

	got := DBURLConnection(dir)
	if _, ok := got["username"]; ok {
		t.Errorf("computed username came across as %q", got["username"])
	}
	if v, ok := got["password"]; !ok || v != "" {
		t.Errorf("a literal empty password must stay: %q %v", v, ok)
	}
	if got["dsn"] != "mysql:host=lerd-mysql;dbname=shop" {
		t.Errorf("dsn = %q", got["dsn"])
	}
}
