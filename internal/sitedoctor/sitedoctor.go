// Package sitedoctor runs Laravel app-level health checks for a single site:
// APP_KEY, .env drift against .env.example, the APP_DEBUG-in-production footgun,
// the public/storage symlink, and pending migrations. The checks are pure
// (files plus one optional artisan exec) so both the web dashboard and the TUI
// can share one implementation rather than each carrying its own copy.
package sitedoctor

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// Check statuses, mirroring the MCP doctor's check shape so the diagnostics
// read consistently. "unknown" covers a check lerd couldn't run (e.g. the app
// is down), which is distinct from a genuine pass or failure.
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusFail    = "fail"
	StatusUnknown = "unknown"
)

// migrateStatusTimeout bounds the one container exec the doctor makes. Booting
// Laravel + reaching the DB is usually sub-second, but a wedged app or an
// unreachable DB shouldn't hang the panel — it degrades to an "unknown" check.
const migrateStatusTimeout = 25 * time.Second

// Check is one app-level health finding for a site. Fix, when set, names a
// command from the site's command set that a UI can run to resolve the finding,
// so the doctor never grows its own mutation endpoints.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// Response is the full doctor report for a site.
type Response struct {
	Checks   []Check `json:"checks"`
	Failures int     `json:"failures"`
	Warnings int     `json:"warnings"`
}

func (d *Response) add(c Check) {
	switch c.Status {
	case StatusFail:
		d.Failures++
	case StatusWarn:
		d.Warnings++
	}
	d.Checks = append(d.Checks, c)
}

// Run builds the doctor report for the Laravel project at path. The cheap
// checks read files only; the migrations check is the one that touches the
// container. Checks that don't apply to a project (no .env.example, no public
// disk) are omitted rather than reported as passing.
func Run(ctx context.Context, path string) Response {
	resp := Response{Checks: []Check{}}
	envPath := filepath.Join(path, ".env")

	resp.add(checkAppKey(envPath))
	if c, ok := checkEnvDrift(path, envPath); ok {
		resp.add(c)
	}
	resp.add(checkAppDebug(envPath))
	if c, ok := checkStorageLink(path); ok {
		resp.add(c)
	}
	resp.add(checkMigrations(ctx, path))
	return resp
}

// checkAppKey fails when APP_KEY is unset, which breaks encryption, signed
// URLs, and session cookies.
func checkAppKey(envPath string) Check {
	if strings.TrimSpace(envfile.ReadKey(envPath, "APP_KEY")) == "" {
		return Check{
			Name:   "app_key",
			Status: StatusFail,
			Detail: "APP_KEY is empty — encryption, signed URLs, and sessions won't work until it's set.",
			Fix:    "key:generate",
		}
	}
	return Check{Name: "app_key", Status: StatusOK}
}

// checkEnvDrift warns when .env.example declares keys the project's .env is
// missing — the classic "pulled main, app breaks on a new env var" trap. Only
// key names are surfaced, never values, so it's safe to return over the wire.
// Skipped (ok=false) when there's no .env.example to compare against.
func checkEnvDrift(path, envPath string) (Check, bool) {
	examplePath := filepath.Join(path, ".env.example")
	if _, err := os.Stat(examplePath); err != nil {
		return Check{}, false
	}
	exampleKeys, err := envfile.ReadKeys(examplePath)
	if err != nil {
		return Check{}, false
	}
	have := map[string]bool{}
	if envKeys, err := envfile.ReadKeys(envPath); err == nil {
		for _, k := range envKeys {
			have[k] = true
		}
	}
	var missing []string
	for _, k := range exampleKeys {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return Check{Name: "env_drift", Status: StatusOK}, true
	}
	// Not every missing key matters: Laravel only breaks on keys read with no
	// default (env('KEY') vs env('KEY', 'fallback')). Split on that signal so
	// the warning fires only for keys the app genuinely needs.
	required, optional := classifyMissingEnvKeys(path, missing)
	if len(required) == 0 {
		return Check{
			Name:   "env_drift",
			Status: StatusOK,
			Detail: fmt.Sprintf("%d key(s) in .env.example aren't set, but the app reads them with defaults, so this is fine.", len(optional)),
		}, true
	}
	detail := fmt.Sprintf("%d required key(s) missing from .env: %s", len(required), summariseKeys(required, 12))
	if len(optional) > 0 {
		detail += fmt.Sprintf(" (%d more have code defaults and were skipped)", len(optional))
	}
	return Check{
		Name:   "env_drift",
		Status: StatusWarn,
		Detail: detail,
	}, true
}

// envCallRe matches a Laravel env('KEY'...) read, capturing the key and the
// char after it: ',' means a default follows (optional), ')' means none does
// (required). The \b stops getenv( and app_env( from matching.
var envCallRe = regexp.MustCompile(`\benv\(\s*['"]([A-Za-z0-9_]+)['"]\s*([,)])`)

// envKeyUsage records how the project's code reads one env key: whether it's
// ever read without a default (so a missing value really breaks something).
type envKeyUsage struct {
	noDefault bool
}

// projectSkipDirs are directories the doctor's source scans never descend into.
var projectSkipDirs = map[string]bool{"vendor": true, "node_modules": true, ".git": true, "storage": true}

// walkProjectSource walks path and invokes onFile with the contents of every
// file whose lowercased extension (without the dot) is in exts, skipping the
// vendor/build/VCS dirs plus any names in extraSkip. Best-effort: walk and
// per-file read errors are ignored.
func walkProjectSource(path string, exts map[string]bool, extraSkip map[string]bool, onFile func(data []byte)) {
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if projectSkipDirs[d.Name()] || extraSkip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(d.Name())), ".")
		if !exts[ext] {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil {
			onFile(data)
		}
		return nil
	})
}

// scanEnvUsage walks the project's PHP for env() reads (skipping vendor and
// build dirs) and returns per-key usage plus the total calls found, so the
// caller can fall back to warning on everything when the scan finds nothing.
func scanEnvUsage(path string) (map[string]envKeyUsage, int) {
	usage := map[string]envKeyUsage{}
	total := 0
	walkProjectSource(path, map[string]bool{"php": true}, nil, func(data []byte) {
		for _, m := range envCallRe.FindAllStringSubmatch(string(data), -1) {
			total++
			u := usage[m[1]]
			if m[2] == ")" {
				u.noDefault = true
			}
			usage[m[1]] = u
		}
	})
	return usage, total
}

// classifyMissingEnvKeys splits missing keys into required (read with no
// default, or a VITE_ key the frontend actually references) and optional (read
// only with defaults, unreferenced, or a VITE_ key nothing in the JS uses). A
// VITE_ key is primarily judged by the frontend scan (Vite reads it via
// import.meta.env), but a VITE_ key also read by PHP env() without a default
// still counts as required; non-VITE keys fall back to "all required" when no
// env() call is found at all.
func classifyMissingEnvKeys(path string, missing []string) (required, optional []string) {
	usage, total := scanEnvUsage(path)
	var viteRefs map[string]bool
	for _, k := range missing {
		switch {
		case strings.HasPrefix(k, "VITE_"):
			if viteRefs == nil {
				viteRefs = scanViteEnvRefs(path)
			}
			// A VITE_ key is usually consumed by the JS bundler (import.meta.env),
			// but it can also be read by PHP via env('VITE_…'); treat a JS source
			// reference or a no-default PHP read as required, otherwise optional
			// (e.g. only present in a compiled public/ bundle).
			if viteRefs[k] || usage[k].noDefault {
				required = append(required, k)
			} else {
				optional = append(optional, k)
			}
		case total == 0:
			required = append(required, k)
		case usage[k].noDefault:
			required = append(required, k)
		default:
			optional = append(optional, k)
		}
	}
	return required, optional
}

// viteKeyRe matches a VITE_-prefixed env identifier referenced in frontend code.
var viteKeyRe = regexp.MustCompile(`VITE_[A-Za-z0-9_]+`)

// viteSourceExts are the frontend file types that can reference import.meta.env.
var viteSourceExts = map[string]bool{
	"js": true, "mjs": true, "cjs": true, "ts": true,
	"jsx": true, "tsx": true, "vue": true, "svelte": true,
}

// scanViteEnvRefs walks the project's frontend source and returns the set of
// VITE_ env keys it references. VITE_ vars are read in JS through
// import.meta.env, never PHP env(), so a missing VITE_ key only matters when the
// frontend uses it — this stops the doctor flagging a stale VITE_ entry in
// .env.example that nothing reads.
func scanViteEnvRefs(path string) map[string]bool {
	refs := map[string]bool{}
	// Skip public/: Vite compiles bundles there with VITE_* literals inlined, so a
	// genuinely stale key would look referenced and defeat the env-drift refinement.
	walkProjectSource(path, viteSourceExts, map[string]bool{"public": true}, func(data []byte) {
		for _, m := range viteKeyRe.FindAllString(string(data), -1) {
			refs[m] = true
		}
	})
	return refs
}

// summariseKeys joins keys for a detail line, capping the list so a project
// with dozens of missing keys doesn't produce a runaway message.
func summariseKeys(keys []string, max int) string {
	if len(keys) <= max {
		return strings.Join(keys, ", ")
	}
	return strings.Join(keys[:max], ", ") + fmt.Sprintf(", +%d more", len(keys)-max)
}

// checkAppDebug warns about the production footgun of APP_DEBUG=true while
// APP_ENV=production, which leaks stack traces. Plain local dev (APP_ENV=local
// with debug on) is expected and passes quietly.
func checkAppDebug(envPath string) Check {
	env := strings.ToLower(strings.TrimSpace(envfile.ReadKey(envPath, "APP_ENV")))
	debug := strings.ToLower(strings.TrimSpace(envfile.ReadKey(envPath, "APP_DEBUG")))
	debugOn := debug == "true" || debug == "1" || debug == "on" || debug == "yes"
	if env == "production" && debugOn {
		return Check{
			Name:   "app_debug",
			Status: StatusWarn,
			Detail: "APP_DEBUG is on while APP_ENV=production — stack traces and config will leak. Turn debug off.",
		}
	}
	return Check{Name: "app_debug", Status: StatusOK}
}

// checkStorageLink warns when a project that uses the public disk
// (storage/app/public exists) is missing its public/storage symlink, so served
// uploads 404. Skipped (ok=false) for apps with no public disk or no public/
// dir, where the symlink is irrelevant.
func checkStorageLink(path string) (Check, bool) {
	link := filepath.Join(path, "public", "storage")
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return Check{Name: "storage_link", Status: StatusOK}, true
	}
	if info, err := os.Stat(filepath.Join(path, "storage", "app", "public")); err != nil || !info.IsDir() {
		return Check{}, false
	}
	if info, err := os.Stat(filepath.Join(path, "public")); err != nil || !info.IsDir() {
		return Check{}, false
	}
	return Check{
		Name:   "storage_link",
		Status: StatusWarn,
		Detail: "public/storage symlink is missing — files on the public disk won't be web-accessible.",
		Fix:    "storage:link",
	}, true
}

// checkMigrations execs `php artisan migrate:status` in the site's container.
// It fails on pending migrations, passes when all have run, and degrades to
// "unknown" when the command can't run (app down, DB unreachable) so a wedged
// app never turns the whole panel into an error.
func checkMigrations(ctx context.Context, path string) Check {
	cctx, cancel := context.WithTimeout(ctx, migrateStatusTimeout)
	defer cancel()
	out, exit, err := runArtisanCapture(cctx, path, "php artisan migrate:status")
	if err != nil || exit != 0 {
		return Check{
			Name:   "migrations",
			Status: StatusUnknown,
			Detail: "Couldn't read migration status — the app may be down or the database unreachable.",
		}
	}
	if migrationsPending(out) {
		return Check{
			Name:   "migrations",
			Status: StatusFail,
			Detail: "There are pending migrations — run migrate to apply them.",
			Fix:    "migrate",
		}
	}
	return Check{Name: "migrations", Status: StatusOK}
}

// migrationsPending reports whether `migrate:status` output lists any not-yet-
// run migration. Laravel marks those rows "Pending" across the supported
// versions; the header carries no such token.
func migrationsPending(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Pending") {
			return true
		}
	}
	return false
}

// runArtisanCapture runs a shell command in cwd with lerd's bin shims on PATH
// (so `php` resolves to the container shim under launchd's restricted PATH),
// mirroring the command runner. Returns combined output and the exit code; a
// non-ExitError (couldn't even start) comes back as exit -1 with the error.
func runArtisanCapture(ctx context.Context, cwd, command string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = cwd
	path := config.BinDir()
	if existing := os.Getenv("PATH"); existing != "" {
		path += string(os.PathListSeparator) + existing
	}
	cmd.Env = append(os.Environ(), "PATH="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode(), nil
		}
		return string(out), -1, err
	}
	return string(out), 0, nil
}
