package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnv(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAppKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Missing key → fail with a key:generate fix.
	writeEnv(t, dir, ".env", "APP_NAME=Acme\nAPP_KEY=\n")
	if c := checkAppKey(envPath); c.Status != StatusFail || c.Fix != "key:generate" {
		t.Errorf("empty APP_KEY: got status=%q fix=%q, want fail/key:generate", c.Status, c.Fix)
	}

	// Set key → ok, no fix.
	writeEnv(t, dir, ".env", "APP_KEY=base64:abcdef==\n")
	if c := checkAppKey(envPath); c.Status != StatusOK || c.Fix != "" {
		t.Errorf("set APP_KEY: got status=%q fix=%q, want ok/none", c.Status, c.Fix)
	}
}

func TestCheckEnvDrift(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// No .env.example → not applicable.
	writeEnv(t, dir, ".env", "APP_KEY=x\n")
	if _, ok := checkEnvDrift(dir, envPath); ok {
		t.Error("expected drift check skipped when no .env.example")
	}

	// Example declares two keys the .env lacks → warn listing both.
	writeEnv(t, dir, ".env.example", "APP_KEY=\nNEW_ONE=\nNEW_TWO=\n")
	writeEnv(t, dir, ".env", "APP_KEY=x\n")
	c, ok := checkEnvDrift(dir, envPath)
	if !ok || c.Status != StatusWarn {
		t.Fatalf("missing keys: got ok=%v status=%q, want true/warn", ok, c.Status)
	}
	if !strings.Contains(c.Detail, "NEW_ONE") || !strings.Contains(c.Detail, "NEW_TWO") {
		t.Errorf("detail should name the missing keys, got %q", c.Detail)
	}

	// All example keys present → ok.
	writeEnv(t, dir, ".env", "APP_KEY=x\nNEW_ONE=1\nNEW_TWO=2\n")
	if c, ok := checkEnvDrift(dir, envPath); !ok || c.Status != StatusOK {
		t.Errorf("aligned env: got ok=%v status=%q, want true/ok", ok, c.Status)
	}
}

// TestCheckEnvDrift_classifiesRequiredVsOptional: when the project's code reads
// some keys with a default and others without, only the no-default keys (plus
// VITE_* the frontend needs) should drive the warning; keys read with a default
// or never referenced are optional and must not turn the row red.
func TestCheckEnvDrift_classifiesRequiredVsOptional(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	writeEnv(t, dir, ".env.example", "APP_KEY=\nDB_HOST=\nLOG_LEVEL=\nVITE_THING=\nVITE_STALE=\nUNUSED_KEY=\n")
	writeEnv(t, dir, ".env", "") // every example key is missing

	// config/ reads APP_KEY with no default (required) and the other two with
	// defaults (optional). VITE_* and UNUSED_KEY aren't read in PHP.
	mustMkdir(t, filepath.Join(dir, "config"))
	writeEnv(t, dir, filepath.Join("config", "app.php"),
		"<?php return [\n"+
			"  'key' => env('APP_KEY'),\n"+
			"  'host' => env('DB_HOST', '127.0.0.1'),\n"+
			"  'log' => env('LOG_LEVEL', 'debug'),\n"+
			"];\n")

	// The frontend references VITE_THING (required) but not VITE_STALE, a leftover
	// .env.example entry nothing reads (optional, must not turn the row red).
	mustMkdir(t, filepath.Join(dir, "resources", "js"))
	writeEnv(t, dir, filepath.Join("resources", "js", "app.js"),
		"const api = import.meta.env.VITE_THING;\nconsole.log(api);\n")

	c, ok := checkEnvDrift(dir, envPath)
	if !ok || c.Status != StatusWarn {
		t.Fatalf("got ok=%v status=%q, want true/warn", ok, c.Status)
	}
	// Required: APP_KEY (no default) and VITE_THING (referenced in JS).
	if !strings.Contains(c.Detail, "APP_KEY") || !strings.Contains(c.Detail, "VITE_THING") {
		t.Errorf("detail should name required keys APP_KEY and VITE_THING, got %q", c.Detail)
	}
	// Optional keys (including an unreferenced VITE_ key) must not be required.
	for _, opt := range []string{"DB_HOST", "LOG_LEVEL", "UNUSED_KEY", "VITE_STALE"} {
		if strings.Contains(c.Detail, opt) {
			t.Errorf("optional key %q should not appear in the required list: %q", opt, c.Detail)
		}
	}
}

// TestCheckEnvDrift_ignoresCompiledPublicBundle: a VITE_ key that only appears
// inlined in a compiled bundle under public/ must not be classified required —
// the source scan skips public/ so a genuinely stale key stays optional.
func TestCheckEnvDrift_ignoresCompiledPublicBundle(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	writeEnv(t, dir, ".env.example", "VITE_STALE=\n")
	writeEnv(t, dir, ".env", "") // missing

	// The only reference to VITE_STALE is in a compiled bundle, not real source.
	mustMkdir(t, filepath.Join(dir, "public", "build", "assets"))
	writeEnv(t, dir, filepath.Join("public", "build", "assets", "app-abc123.js"),
		"const x=import.meta.env.VITE_STALE;console.log(x);\n")

	refs := scanViteEnvRefs(dir)
	if refs["VITE_STALE"] {
		t.Errorf("VITE_STALE in public/ should be ignored, got referenced")
	}

	c, ok := checkEnvDrift(dir, envPath)
	if !ok {
		t.Fatalf("expected a drift check result")
	}
	if strings.Contains(c.Detail, "VITE_STALE") && c.Status == StatusWarn {
		t.Errorf("a public/-only VITE_ key must not be flagged required: %q", c.Detail)
	}
}

// TestCheckEnvDrift_ViteKeyReadByPhpNoDefault: a VITE_ key that isn't referenced
// in JS but is read by PHP via env('VITE_…') without a default must still be
// classified required, not optional.
func TestCheckEnvDrift_ViteKeyReadByPhpNoDefault(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	writeEnv(t, dir, ".env.example", "VITE_SSR=\n")
	writeEnv(t, dir, ".env", "") // missing

	mustMkdir(t, filepath.Join(dir, "config"))
	writeEnv(t, dir, filepath.Join("config", "app.php"),
		"<?php return ['ssr' => env('VITE_SSR')];\n")

	c, ok := checkEnvDrift(dir, envPath)
	if !ok || c.Status != StatusWarn {
		t.Fatalf("got ok=%v status=%q, want true/warn", ok, c.Status)
	}
	if !strings.Contains(c.Detail, "VITE_SSR") {
		t.Errorf("a VITE_ key read by PHP without a default must be required, got %q", c.Detail)
	}
}

// TestCheckEnvDrift_allOptionalStaysGreen: when every missing key is read with a
// default, the check passes quietly with an informational note instead of
// warning.
func TestCheckEnvDrift_allOptionalStaysGreen(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	writeEnv(t, dir, ".env.example", "DB_HOST=\nLOG_LEVEL=\n")
	writeEnv(t, dir, ".env", "")
	mustMkdir(t, filepath.Join(dir, "config"))
	writeEnv(t, dir, filepath.Join("config", "app.php"),
		"<?php return [\n"+
			"  'host' => env('DB_HOST', '127.0.0.1'),\n"+
			"  'log' => env('LOG_LEVEL', 'debug'),\n"+
			"];\n")

	c, ok := checkEnvDrift(dir, envPath)
	if !ok || c.Status != StatusOK {
		t.Fatalf("all-optional: got ok=%v status=%q, want true/ok", ok, c.Status)
	}
}

func TestCheckAppDebug(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// production + debug on → warn (the footgun).
	writeEnv(t, dir, ".env", "APP_ENV=production\nAPP_DEBUG=true\n")
	if c := checkAppDebug(envPath); c.Status != StatusWarn {
		t.Errorf("prod+debug: got %q, want warn", c.Status)
	}

	// local + debug on → ok (normal dev).
	writeEnv(t, dir, ".env", "APP_ENV=local\nAPP_DEBUG=true\n")
	if c := checkAppDebug(envPath); c.Status != StatusOK {
		t.Errorf("local+debug: got %q, want ok", c.Status)
	}

	// production + debug off → ok.
	writeEnv(t, dir, ".env", "APP_ENV=production\nAPP_DEBUG=false\n")
	if c := checkAppDebug(envPath); c.Status != StatusOK {
		t.Errorf("prod+nodebug: got %q, want ok", c.Status)
	}
}

func TestCheckStorageLink(t *testing.T) {
	// No public disk → not applicable.
	bare := t.TempDir()
	if _, ok := checkStorageLink(bare); ok {
		t.Error("expected skip when there's no storage/app/public")
	}

	// Uses public disk, public/ exists, symlink missing → warn + fix.
	missing := t.TempDir()
	mustMkdir(t, filepath.Join(missing, "storage", "app", "public"))
	mustMkdir(t, filepath.Join(missing, "public"))
	c, ok := checkStorageLink(missing)
	if !ok || c.Status != StatusWarn || c.Fix != "storage:link" {
		t.Errorf("missing link: got ok=%v status=%q fix=%q, want true/warn/storage:link", ok, c.Status, c.Fix)
	}

	// Symlink present → ok regardless of disk layout.
	linked := t.TempDir()
	mustMkdir(t, filepath.Join(linked, "public"))
	mustMkdir(t, filepath.Join(linked, "storage", "app", "public"))
	if err := os.Symlink("../storage/app/public", filepath.Join(linked, "public", "storage")); err != nil {
		t.Fatal(err)
	}
	if c, ok := checkStorageLink(linked); !ok || c.Status != StatusOK {
		t.Errorf("present link: got ok=%v status=%q, want true/ok", ok, c.Status)
	}
}

func TestMigrationsPending(t *testing.T) {
	pending := "  Migration name ....................... Batch / Status\n" +
		"  2014_10_12_000000_create_users_table .. [1] Ran\n" +
		"  2024_01_01_000000_create_orders_table . Pending\n"
	if !migrationsPending(pending) {
		t.Error("expected pending=true when a row is Pending")
	}

	allRan := "  2014_10_12_000000_create_users_table .. [1] Ran\n" +
		"  2019_08_19_000000_create_failed_jobs .. [1] Ran\n"
	if migrationsPending(allRan) {
		t.Error("expected pending=false when every row Ran")
	}
}
