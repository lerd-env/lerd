package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/presetfixtures"
)

func init() { config.SetExtraPresetsForTest(presetfixtures.FS()) }

func withServiceHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
}

func writeDepQuadlet(t *testing.T, unit string) {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, unit+".container"), []byte("[Container]\nImage=x\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDependency_ExactMatch(t *testing.T) {
	withServiceHome(t)
	writeDepQuadlet(t, "lerd-mysql")

	if got := ResolveDependency("mysql"); got != "mysql" {
		t.Errorf("ResolveDependency(mysql) = %q, want mysql", got)
	}
}

func TestResolveDependency_FamilyMember(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "postgres-pgvector-18", Image: "x", Family: "postgres", Preset: "postgres-pgvector",
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDependency("postgres"); got != "postgres-pgvector-18" {
		t.Errorf("ResolveDependency(postgres) = %q, want postgres-pgvector-18", got)
	}
}

func TestResolveDependency_EnvRoleDropIn(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDependency("mysql"); got != "mariadb-11-8" {
		t.Errorf("ResolveDependency(mysql) = %q, want mariadb-11-8", got)
	}
}

func TestResolveDependency_ValkeySatisfiesRedis(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "valkey", Image: "x", Family: "valkey", EnvRole: "redis", Preset: "valkey",
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDependency("redis"); got != "valkey" {
		t.Errorf("ResolveDependency(redis) = %q, want valkey", got)
	}
}

func TestResolveDependency_PrefersLiteral(t *testing.T) {
	withServiceHome(t)
	writeDepQuadlet(t, "lerd-mysql")
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql",
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDependency("mysql"); got != "mysql" {
		t.Errorf("ResolveDependency(mysql) = %q, want mysql when both installed", got)
	}
}

func TestResolveDependency_PrefersFamilyOverEnvRole(t *testing.T) {
	withServiceHome(t)
	// No bare mysql: versioned mysql and mariadb both satisfy. Same-family
	// must win over the env_role drop-in (alphabetical would pick mariadb).
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-12-3", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mysql-9-7", Image: "x", Family: "mysql", Preset: "mysql",
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDependency("mysql"); got != "mysql-9-7" {
		t.Errorf("ResolveDependency(mysql) = %q, want mysql-9-7 (family over env_role)", got)
	}
	if got := DependencyDisplayName("mysql"); got != "mysql" {
		t.Errorf("DependencyDisplayName(mysql) = %q, want mysql", got)
	}
}

func TestResolveDependency_NothingInstalled(t *testing.T) {
	withServiceHome(t)
	if got := ResolveDependency("mysql"); got != "" {
		t.Errorf("ResolveDependency(mysql) = %q, want empty", got)
	}
}

func TestMissingPresetDependencies_EnvRoleDropInOK(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql",
	}); err != nil {
		t.Fatal(err)
	}

	missing := MissingPresetDependencies(&config.CustomService{
		Name: "phpmyadmin", DependsOn: []string{"mysql"},
	})
	if len(missing) != 0 {
		t.Errorf("mariadb should satisfy mysql dep, got missing=%v", missing)
	}
}

func TestMissingPresetDependencies_ValkeyOKForRedisInsight(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "valkey", Image: "x", Family: "valkey", EnvRole: "redis",
	}); err != nil {
		t.Fatal(err)
	}

	missing := MissingPresetDependencies(&config.CustomService{
		Name: "redisinsight", DependsOn: []string{"redis"},
	})
	if len(missing) != 0 {
		t.Errorf("valkey should satisfy redisinsight's redis dep, got missing=%v", missing)
	}
}

func TestMissingPresetDependencies_MentionsAlternatives(t *testing.T) {
	withServiceHome(t)

	missing := MissingPresetDependencies(&config.CustomService{
		Name: "phpmyadmin", DependsOn: []string{"mysql"},
	})
	if len(missing) != 1 {
		t.Fatalf("expected one missing dep, got %v", missing)
	}
	if !strings.Contains(missing[0], "mysql") || !strings.Contains(missing[0], "mariadb") {
		t.Errorf("missing label should mention mysql and mariadb, got %q", missing[0])
	}
}

func TestMissingPresetDependencies_FamilyMemberNoDiscoverFamily(t *testing.T) {
	withServiceHome(t)
	// Same-family member satisfies even when the dependent has no discover_family
	// (mongo-express hard-codes lerd-mongo; dependency gating still accepts mongo-7).
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mongo-7", Image: "x", Family: "mongo",
	}); err != nil {
		t.Fatal(err)
	}

	missing := MissingPresetDependencies(&config.CustomService{
		Name: "mongo-express", DependsOn: []string{"mongo"},
	})
	if len(missing) != 0 {
		t.Errorf("mongo-7 should satisfy mongo dep via family, got missing=%v", missing)
	}
}

func TestDependencyDisplayName_UsesPresetNotVersionedName(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}

	if got := DependencyDisplayName("mysql"); got != "mariadb" {
		t.Errorf("DependencyDisplayName(mysql) = %q, want mariadb", got)
	}
}

func TestDependencyDisplayName_UnsatisfiedKeepsDeclared(t *testing.T) {
	withServiceHome(t)
	if got := DependencyDisplayName("mysql"); got != "mysql" {
		t.Errorf("DependencyDisplayName(mysql) = %q, want mysql", got)
	}
}

func TestDependencyDisplayName_ExactBuiltin(t *testing.T) {
	withServiceHome(t)
	writeDepQuadlet(t, "lerd-mysql")
	if got := DependencyDisplayName("mysql"); got != "mysql" {
		t.Errorf("DependencyDisplayName(mysql) = %q, want mysql", got)
	}
}

func TestDependentNeedsCascade_AlternateRemains(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-12-3", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}
	writeDepQuadlet(t, "lerd-mysql")
	if err := config.SaveCustomService(&config.CustomService{
		Name: "phpmyadmin", Image: "x", DependsOn: []string{"mysql"},
	}); err != nil {
		t.Fatal(err)
	}
	// Both installed and "running" (ServiceRunning nil → installed counts).
	if dependentNeedsCascade("phpmyadmin", "mysql") {
		t.Fatal("cascade must not run when mariadb still satisfies mysql")
	}
	if got := dependentsOf("mysql"); len(got) != 1 || got[0] != "phpmyadmin" {
		t.Errorf("dependentsOf(mysql) = %v, want [phpmyadmin]", got)
	}
}

func TestDependentNeedsCascade_LastSatisfier(t *testing.T) {
	withServiceHome(t)
	writeDepQuadlet(t, "lerd-mysql")
	if err := config.SaveCustomService(&config.CustomService{
		Name: "phpmyadmin", Image: "x", DependsOn: []string{"mysql"},
	}); err != nil {
		t.Fatal(err)
	}

	if !dependentNeedsCascade("phpmyadmin", "mysql") {
		t.Fatal("cascade must run when mysql is the only satisfier")
	}
}

func TestDependentNeedsCascade_StoppedAlternateDoesNotCount(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-12-3", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}
	writeDepQuadlet(t, "lerd-mysql")
	if err := config.SaveCustomService(&config.CustomService{
		Name: "phpmyadmin", Image: "x", DependsOn: []string{"mysql"},
	}); err != nil {
		t.Fatal(err)
	}
	prev := config.ServiceRunning
	config.ServiceRunning = func(name string) bool { return name == "mysql" }
	t.Cleanup(func() { config.ServiceRunning = prev })

	// mariadb is installed but stopped; stopping mysql is the last running
	// satisfier so phpMyAdmin must cascade-stop.
	if !dependentNeedsCascade("phpmyadmin", "mysql") {
		t.Fatal("cascade must run when the only other satisfier is stopped")
	}
}

func TestDependentsOf_EnvRoleDropIn(t *testing.T) {
	withServiceHome(t)
	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-12-3", Image: "x", Family: "mariadb", EnvRole: "mysql",
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveCustomService(&config.CustomService{
		Name: "phpmyadmin", Image: "x", DependsOn: []string{"mysql"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := dependentsOf("mariadb-12-3"); len(got) != 1 || got[0] != "phpmyadmin" {
		t.Errorf("dependentsOf(mariadb) = %v, want [phpmyadmin]", got)
	}
}
