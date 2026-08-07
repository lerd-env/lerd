package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// wiringFramework is a definition shaped like the published ones: it declares
// which services it knows how to wire into env, and by which keys.
func wiringFramework() *config.Framework {
	return &config.Framework{
		Name:  "acme",
		Label: "Acme",
		Env: config.FrameworkEnvConf{
			File: ".env",
			Services: map[string]config.FrameworkServiceDef{
				"mysql":    {Vars: []string{"DB_HOST=lerd-mysql", "DB_NAME={{site}}"}},
				"postgres": {Vars: []string{"DB_HOST=lerd-postgres", "DB_NAME={{site}}"}},
				"redis":    {Vars: []string{"REDIS_HOST=lerd-redis"}},
			},
		},
	}
}

func wiringProject(t *testing.T, lerdYAML, envFile, envBody string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	if lerdYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(lerdYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if envFile != "" {
		path := filepath.Join(dir, envFile)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(envBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The drupal case: the project picks mysql and its config points at nothing of
// the sort, and every check that would have noticed sits behind a dotenv gate
// this project's config never passes.
func TestCheckServiceWiring_reportsAPickedServiceNothingPointsAt(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mysql\n", "settings.php",
		"<?php\n$databases['default']['default'] = ['driver' => 'sqlite'];\n")

	c, ok := checkServiceWiring(dir, "settings.php", wiringFramework())
	if !ok {
		t.Fatal("no wiring check produced")
	}
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "mysql") || !strings.Contains(c.Detail, "settings.php") {
		t.Errorf("detail = %q, want the service and the file named", c.Detail)
	}
}

// A wired project passes, whatever format its config is in: the question is
// whether the file references the container, which every format answers.
func TestCheckServiceWiring_passesWhenTheConfigPointsAtIt(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mysql\n", "wp-config.php",
		"<?php\ndefine( 'DB_HOST', 'lerd-mysql' );\n")

	c, ok := checkServiceWiring(dir, "wp-config.php", wiringFramework())
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok", c, ok)
	}
}

// A service the framework does not wire is picked for its own sake and is
// never expected in an env file.
func TestCheckServiceWiring_ignoresAServiceTheFrameworkDoesNotWire(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mysql\n  - phpmyadmin\n", ".env", "DB_HOST=lerd-mysql\n")

	c, ok := checkServiceWiring(dir, ".env", wiringFramework())
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok with phpmyadmin ignored", c, ok)
	}
}

// A project running its own instance says so in .env.lerd_override and
// deliberately does not reference the container.
func TestCheckServiceWiring_ignoresAnExternallyManagedService(t *testing.T) {
	dir := wiringProject(t, "services:\n  - postgres\n", ".env", "DB_HOST=127.0.0.1\n")
	if err := os.WriteFile(filepath.Join(dir, ".env.lerd_override"), []byte("LERD_EXTERNAL_SERVICES=postgres\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, ok := checkServiceWiring(dir, ".env", wiringFramework())
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok for an externally managed service", c, ok)
	}
}

// sqlite has no container to reference.
func TestCheckServiceWiring_ignoresSqlite(t *testing.T) {
	dir := wiringProject(t, "services:\n  - sqlite\n", ".env", "DB_CONNECTION=sqlite\n")

	c, ok := checkServiceWiring(dir, ".env", wiringFramework())
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok for sqlite", c, ok)
	}
}

// installDropIn registers a service that stands in for another, the way an
// installed mariadb preset declares itself a drop-in for mysql.
func installDropIn(t *testing.T, name, family, role string) {
	t.Helper()
	if err := config.SaveCustomService(&config.CustomService{
		Name:    name,
		Image:   "docker.io/library/mariadb:11.8",
		Family:  family,
		EnvRole: role,
	}); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}
}

// A drop-in stands in for the service the framework wires, so a project on
// mariadb is wired when its config points at that container.
func TestCheckServiceWiring_acceptsADropInForAWiredFamily(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mariadb-11-8\n", ".env", "DB_HOST=lerd-mariadb-11-8\n")
	installDropIn(t, "mariadb-11-8", "mariadb", "mysql")

	c, ok := checkServiceWiring(dir, ".env", wiringFramework())
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok for a drop-in", c, ok)
	}
}

func TestCheckServiceWiring_reportsAnUnwiredDropIn(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mariadb-11-8\n", ".env", "DB_HOST=127.0.0.1\n")
	installDropIn(t, "mariadb-11-8", "mariadb", "mysql")

	c, ok := checkServiceWiring(dir, ".env", wiringFramework())
	if !ok || c.Status != StatusWarn || !strings.Contains(c.Detail, "mariadb-11-8") {
		t.Errorf("check = %+v (ok=%v), want a warning naming mariadb-11-8", c, ok)
	}
}

// Nothing declared, nothing to drift from. A framework that wires nothing is
// the same: there is no declaration to measure the project against.
func TestCheckServiceWiring_skipsWhenThereIsNothingToCompare(t *testing.T) {
	if c, ok := checkServiceWiring(wiringProject(t, "", ".env", ""), ".env", wiringFramework()); ok {
		t.Errorf("check = %+v, want none without a .lerd.yaml", c)
	}
	dir := wiringProject(t, "services:\n  - mysql\n", ".env", "")
	if c, ok := checkServiceWiring(dir, ".env", &config.Framework{Name: "static"}); ok {
		t.Errorf("check = %+v, want none for a framework that wires nothing", c)
	}
}

// A missing env file is the env-present check's finding, not this one's, and
// reporting both would say the same thing twice.
func TestCheckServiceWiring_skipsWhenTheEnvFileIsMissing(t *testing.T) {
	dir := wiringProject(t, "services:\n  - mysql\n", "", "")

	if c, ok := checkServiceWiring(dir, ".env", wiringFramework()); ok {
		t.Errorf("check = %+v, want none when the env file is absent", c)
	}
}
