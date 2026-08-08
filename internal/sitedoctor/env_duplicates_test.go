package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A key set twice means different things to different runtimes, and lerd
// reading the first can disagree with the application about which database it
// is on. That was a site reporting postgres on every surface while running on
// SQLite, with nothing anywhere saying why.
func TestCheckEnvDuplicates_reportsAKeySetTwice(t *testing.T) {
	dir := envFile(t, "DATABASE_URL=postgresql://postgres@lerd-postgres:5432/app\nAPP_ENV=dev\nDATABASE_URL=\"sqlite:///var/app.db\"\n")

	c, ok := checkEnvDuplicates(dir, ".env", "dotenv")
	if !ok || c.Status != StatusWarn {
		t.Fatalf("check = %+v (ok=%v), want a warning", c, ok)
	}
	for _, want := range []string{"DATABASE_URL", "postgresql://", "sqlite:///var/app.db", "set 2 times"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail = %q, want it to mention %q", c.Detail, want)
		}
	}
}

// A value left behind under a comment is not a second setting.
func TestCheckEnvDuplicates_ignoresCommentedValues(t *testing.T) {
	dir := envFile(t, "# DATABASE_URL=mysql://old\nDATABASE_URL=postgresql://new\n")

	if c, _ := checkEnvDuplicates(dir, ".env", "dotenv"); c.Status != StatusOK {
		t.Errorf("check = %+v, want ok: the other value is commented out", c)
	}
}

func TestCheckEnvDuplicates_passesForAFileThatSaysEachThingOnce(t *testing.T) {
	dir := envFile(t, "APP_ENV=dev\nDATABASE_URL=postgresql://new\n")

	if c, ok := checkEnvDuplicates(dir, ".env", "dotenv"); !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok", c, ok)
	}
}

// A PHP file's duplicate has one answer the language decides, and lerd reads it
// the way PHP runs it, so there is no ambiguity to report.
func TestCheckEnvDuplicates_skipsFormatsThatAreNotAmbiguous(t *testing.T) {
	dir := envFile(t, "irrelevant\n")
	for _, format := range []string{"php-const", "php-vars", "php-array"} {
		if c, ok := checkEnvDuplicates(dir, ".env", format); ok {
			t.Errorf("%s: check = %+v, want none", format, c)
		}
	}
}
