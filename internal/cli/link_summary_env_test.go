package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The first link of a project is exactly when .env.before_lerd is written, so a
// summary that preferred the backup reported the database lerd had just
// replaced rather than the one it configured (#1144).
func TestSummaryEnvReader_readsTheLiveEnvNotTheBackup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "DB_CONNECTION=mysql\nCACHE_STORE=redis\n")
	writeFile(t, dir, ".env.before_lerd", "DB_CONNECTION=sqlite\nCACHE_STORE=database\n")

	read := summaryEnvReader(config.Site{Path: dir})
	if got := read("DB_CONNECTION"); got != "mysql" {
		t.Errorf("DB_CONNECTION = %q, want mysql (the live env, not the pre-lerd backup)", got)
	}
	if got := read("CACHE_STORE"); got != "redis" {
		t.Errorf("CACHE_STORE = %q, want redis", got)
	}
}

func TestSummaryEnvReader_fallsBackToTheExampleBeforeTheEnvExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", "DB_CONNECTION=pgsql\n")

	if got := summaryEnvReader(config.Site{Path: dir})("DB_CONNECTION"); got != "pgsql" {
		t.Errorf("DB_CONNECTION = %q, want pgsql from the example file", got)
	}
}

func TestSummaryEnvReader_emptyWhenThereIsNoEnvAtAll(t *testing.T) {
	if got := summaryEnvReader(config.Site{Path: t.TempDir()})("DB_CONNECTION"); got != "" {
		t.Errorf("DB_CONNECTION = %q, want empty", got)
	}
}
