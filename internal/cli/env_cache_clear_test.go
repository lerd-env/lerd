package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func cacheFW() *config.Framework {
	return &config.Framework{
		Console:      "vendor/drush/drush/drush.php",
		CacheCommand: "cr",
		Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
			"mysql": {Vars: []string{
				"databases.default.default.host=lerd-mysql",
				"databases.default.default.database={{site}}",
			}},
			"mailpit": {Vars: []string{"SMTP_HOST=lerd-mailpit"}},
		}},
	}
}

// The cache is cleared because the site moved database, so only a database key
// counts. A rewritten URL or mail setting must not trigger it, or every env run
// pays for a cache rebuild it did not need.
func TestDBConnectionKey_onlyDatabaseKeysCount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fw := cacheFW()

	for _, key := range []string{"databases.default.default.host", "databases.default.default.database"} {
		if !dbConnectionKey(fw, key) {
			t.Errorf("%s should count as a connection key", key)
		}
	}
	for _, key := range []string{"SMTP_HOST", "APP_URL", "databases.default.default.unknown"} {
		if dbConnectionKey(fw, key) {
			t.Errorf("%s should not count as a connection key", key)
		}
	}
}

// A framework that declares no database service has no connection keys, so
// nothing it writes ever triggers a clear.
func TestDBConnectionKey_frameworkWithoutADatabase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fw := &config.Framework{Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
		"mailpit": {Vars: []string{"SMTP_HOST=lerd-mailpit"}},
	}}}
	if dbConnectionKey(fw, "SMTP_HOST") {
		t.Error("a mail key counted as a database connection")
	}
}
