// Package sitedoctor runs app-level health checks for a single site. It pairs a
// universal baseline every framework gets (env file present, .env drift,
// dependency install + lock, security audits, PHP version range) with the
// framework's own declarative checks from the store (config.FrameworkDoctor), so
// the web dashboard, TUI, and CLI all share one framework-agnostic engine.
package sitedoctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	phpkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
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

// Universal doctor fix keys. A Check.Fix set to one of these names a package
// manager command the UI runs through the doctor fix endpoint, distinct from a
// framework command from the site's own command set.
const (
	FixComposerInstall = "composer_install"
	FixComposerUpdate  = "composer_update"
	FixNpmInstall      = "npm_install"
	FixNpmAuditFix     = "npm_audit_fix"
)

// DoctorFixCommands maps each universal fix key to the shell command run in the
// site container. The fix endpoint only runs commands from this allowlist, so a
// client can never drive arbitrary shell through it.
var DoctorFixCommands = map[string]string{
	FixComposerInstall: "composer install",
	FixComposerUpdate:  "composer update",
	FixNpmInstall:      "npm install",
	FixNpmAuditFix:     "npm audit fix",
}

// commandTimeout bounds each container exec a check makes (declared command
// checks plus the composer/npm audits). A wedged app, unreachable DB, or slow
// network degrades the check to "unknown" rather than hanging the panel.
const commandTimeout = 25 * time.Second

// Check is one app-level health finding for a site. Fix, when set, names a
// command from the site's command set that a UI can run to resolve the finding,
// so the doctor never grows its own mutation endpoints.
type Check struct {
	Name   string `json:"name"`
	Label  string `json:"label,omitempty"`
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

// Run builds the doctor report for the project at path using fw to drive both
// the universal baseline and the framework's declarative checks. fw may be nil
// (an unknown framework) — only the file/dependency baseline runs then. The
// cheap checks read files; command and audit checks touch the container.
// RunForPath resolves the framework definition for a project path and runs the
// doctor, so the CLI, MCP, and Web UI share one path -> framework -> Run chain
// instead of re-deriving it three ways. fwName is the site's recorded framework
// when known; pass "" to detect it from the path.
func RunForPath(ctx context.Context, path, fwName string) Response {
	if fwName == "" {
		fwName, _ = config.DetectFrameworkForDir(path)
	}
	fw, _ := config.GetFrameworkForDir(fwName, path)
	return Run(ctx, path, fw)
}

// AppliesForPath reports whether the doctor has any check to run for the project
// at path, resolving the framework the same way RunForPath does so the dashboard,
// CLI, and MCP share one applicability answer.
func AppliesForPath(path, fwName string) bool {
	if fwName == "" {
		fwName, _ = config.DetectFrameworkForDir(path)
	}
	fw, _ := config.GetFrameworkForDir(fwName, path)
	return Applies(path, fw)
}

// Applies reports whether Run would have anything to check, without touching the
// container. It mirrors Run's gating from the cheap, config-and-manifest signals
// so a site the report would leave empty (a host-proxy Python, Ruby, or Go dev
// server: no framework, no composer.json, no package.json) can hide the doctor
// button instead of opening a modal that finds nothing. A Node host-proxy keeps
// it, since its package.json still drives the node checks.
func Applies(path string, fw *config.Framework) bool {
	if fw != nil {
		if len(fw.Requires) > 0 || fw.HasEnvConfig() || len(frameworkChecks(fw)) > 0 {
			return true
		}
		if fw.PHP.Min != "" || fw.PHP.Max != "" {
			return true
		}
	}
	if fileExists(filepath.Join(path, "composer.json")) && !composerDisabled(fw) {
		return true
	}
	if fileExists(filepath.Join(path, "package.json")) {
		return true
	}
	// A committed dotenv example drives the env_drift check even with no framework,
	// so a bare proxy carrying one still has something to report.
	if _, format, exampleFile := envSetup(fw, path); format == "dotenv" && fileExists(filepath.Join(path, exampleFile)) {
		return true
	}
	return false
}

func Run(ctx context.Context, path string, fw *config.Framework) Response {
	resp := Response{Checks: []Check{}}
	envFile, envFormat, exampleFile := envSetup(fw, path)
	envPath := filepath.Join(path, envFile)

	if c, ok := checkRequiredServices(fw); ok {
		resp.add(c)
	}
	if hasEnvConfig(fw) {
		if c, ok := checkEnvPresent(path, envFile, fwExampleFile(fw)); ok {
			resp.add(c)
		}
	}
	// The env drift and app-key checks parse the file as dotenv, so skip them for
	// frameworks that store config another way (WordPress's wp-config.php, etc.).
	dbBroken := false
	if envFormat == "dotenv" {
		if c, ok := checkAppKey(envPath, fw); ok {
			resp.add(c)
		}
		if c, ok := checkEnvDrift(path, envPath, filepath.Join(path, exampleFile)); ok {
			resp.add(c)
		}
		if c, ok := checkSQLiteDatabase(path, envPath, fw); ok {
			resp.add(c)
			dbBroken = c.Status == StatusFail
		}
	}

	// The framework command checks and the composer/node dependency + audit checks
	// each block on an independent container-exec timeout. Run them concurrently
	// (results collected in order) so a down app caps the panel at roughly one
	// timeout instead of the sum of them all.
	var tasks []func() (Check, bool)
	for _, spec := range frameworkChecks(fw) {
		spec := spec
		// A known-broken database turns a migration check into "couldn't run" noise
		// that just repeats the database finding's remedy, so skip a command check
		// whose fix is the same migrate command; unrelated command checks still run.
		if dbBroken && spec.Type == "command" && spec.Fix == sqliteFixCommand {
			continue
		}
		tasks = append(tasks, func() (Check, bool) { return runDeclaredCheck(ctx, path, envPath, envFormat, spec) })
	}
	tasks = append(tasks, dependencyCheckTasks(ctx, path, fw)...)
	for _, c := range runChecksConcurrently(tasks) {
		resp.add(c)
	}
	if c, ok := checkPHPVersion(path, fw); ok {
		resp.add(c)
	}
	if c, ok := checkSlowRoutes(path); ok {
		resp.add(c)
	}
	applyLabels(&resp)
	return resp
}

// envSetup resolves the env file, its format, and the example file for the
// framework. It honours the framework's fallback (WordPress resolves to
// wp-config.php), defaulting to .env / dotenv / .env.example when fw is nil.
func envSetup(fw *config.Framework, path string) (envFile, format, exampleFile string) {
	envFile, format, exampleFile = ".env", "dotenv", ".env.example"
	if fw != nil {
		envFile, format = fw.Env.Resolve(path)
		if fw.Env.ExampleFile != "" {
			exampleFile = fw.Env.ExampleFile
		}
	}
	return envFile, format, exampleFile
}

// hasEnvConfig reports whether the framework uses an env file at all, so a plain
// site with no framework isn't flagged for a missing .env it never needed.
func hasEnvConfig(fw *config.Framework) bool { return fw.HasEnvConfig() }

// fwExampleFile returns the framework's declared example file, or "" when it has
// none (Magento's env.php is generated, never copied from a template).
func fwExampleFile(fw *config.Framework) string {
	if fw == nil {
		return ""
	}
	return fw.Env.ExampleFile
}

// Seams so the required-service check can be tested without podman.
var (
	quadletInstalledFn = podman.QuadletInstalled
	unitStatusFn       = podman.UnitStatus
)

// checkRequiredServices reports the framework's declared required services that
// are absent or stopped. Absent is a failure, since the app cannot boot without
// it; stopped is a warning, since starting it is one command.
func checkRequiredServices(fw *config.Framework) (Check, bool) {
	if fw == nil || len(fw.Requires) == 0 {
		return Check{}, false
	}
	var missing, stopped []string
	for _, name := range fw.Requires {
		unit := "lerd-" + name
		if !quadletInstalledFn(unit) {
			missing = append(missing, name)
			continue
		}
		if status, _ := unitStatusFn(unit); status != "active" {
			stopped = append(stopped, name)
		}
	}
	switch {
	case len(missing) > 0:
		return Check{
			Name:   "required_services",
			Status: StatusFail,
			Detail: fmt.Sprintf("%s cannot run without %s. Install %s with %s.",
				frameworkLabel(fw), strings.Join(missing, ", "), plural(len(missing), "it", "them"),
				serviceCommands("lerd service preset", missing)),
		}, true
	case len(stopped) > 0:
		return Check{
			Name:   "required_services",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s %s required but not running. Start %s with %s.",
				strings.Join(stopped, ", "), plural(len(stopped), "is", "are"),
				plural(len(stopped), "it", "them"),
				serviceCommands("lerd service start", stopped)),
		}, true
	}
	return Check{Name: "required_services", Status: StatusOK}, true
}

// serviceCommands renders the command to run for every named service, not just
// the first: the suggested fix has to actually resolve the check.
func serviceCommands(base string, names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "'"+base+" "+n+"'")
	}
	return strings.Join(quoted, " and ")
}

// frameworkLabel prefers the display label, falling back to the slug.
func frameworkLabel(fw *config.Framework) string {
	if fw.Label != "" {
		return fw.Label
	}
	return fw.Name
}

// frameworkChecks returns the framework's declarative doctor checks, or nil.
func frameworkChecks(fw *config.Framework) []config.DoctorCheck {
	if fw == nil || fw.Doctor == nil {
		return nil
	}
	return fw.Doctor.Checks
}

// runDeclaredCheck dispatches one store-declared check to its typed evaluator,
// stamping the spec's label. An unknown type is skipped (ok=false) so a newer
// store never errors an older binary.
func runDeclaredCheck(ctx context.Context, path, envPath, envFormat string, spec config.DoctorCheck) (Check, bool) {
	// The declared env checks must read the file in the framework's own format,
	// or a php-const / php-array config silently reads as an empty dotenv.
	read := envfile.Reader(envPath, envFormat)
	var c Check
	var ok bool
	switch spec.Type {
	case "env_key_set":
		c, ok = checkEnvKeySet(read, spec), true
	case "env_combo":
		c, ok = checkEnvCombo(read, spec), true
	case "symlink":
		c, ok = checkSymlink(path, spec)
	case "command":
		c, ok = checkCommand(ctx, path, spec), true
	default:
		return Check{}, false
	}
	if ok {
		c.Label = spec.Label
	}
	return c, ok
}

// universalLabels maps the built-in check names to their display labels. The
// declared framework checks carry their own labels from the store.
var universalLabels = map[string]string{
	"required_services": "Required Services",
	"env_present":       "Env File",
	"app_key":           "App Key",
	"env_drift":         "Env Drift",
	"sqlite_database":   "Database",
	"composer_deps":     "Composer Dependencies",
	"composer_audit":    "Composer Audit",
	"node_deps":         "Node Dependencies",
	"node_audit":        "Node Audit",
	"php_version":       "PHP Version",
	"slow_routes":       "Response Time",
}

// humanize turns a snake_case check name into a Title Case fallback label.
func humanize(name string) string {
	words := strings.Split(name, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// applyLabels fills any check whose Label is still empty, preferring the
// universal label table and falling back to a humanized name.
func applyLabels(resp *Response) {
	for i := range resp.Checks {
		if resp.Checks[i].Label != "" {
			continue
		}
		if l, ok := universalLabels[resp.Checks[i].Name]; ok {
			resp.Checks[i].Label = l
		} else {
			resp.Checks[i].Label = humanize(resp.Checks[i].Name)
		}
	}
}

// checkEnvPresent fails when the framework's env file is missing — every other
// env-driven check would otherwise read an empty file and misreport.
func envMissingDetail(envFile, exampleFile string) string {
	if exampleFile == "" {
		return fmt.Sprintf("%s is missing.", envFile)
	}
	return fmt.Sprintf("%s is missing, copy it from the example and configure it.", envFile)
}

func checkEnvPresent(path, envFile, exampleFile string) (Check, bool) {
	if _, err := os.Stat(filepath.Join(path, envFile)); err != nil {
		return Check{
			Name:   "env_present",
			Status: StatusFail,
			Detail: envMissingDetail(envFile, exampleFile),
		}, true
	}
	return Check{Name: "env_present", Status: StatusOK}, true
}

// checkAppKey fails when the framework's app key (env.key_generation) is unset,
// which breaks encryption, signed URLs, and session cookies. Skipped for
// frameworks that declare no key generation.
func checkAppKey(envPath string, fw *config.Framework) (Check, bool) {
	if fw == nil || fw.Env.KeyGeneration == nil || fw.Env.KeyGeneration.EnvKey == "" {
		return Check{}, false
	}
	kg := fw.Env.KeyGeneration
	// An empty app key breaks encryption, signed URLs and sessions, so this one
	// fails rather than taking the declared-check default of warn.
	spec := config.DoctorCheck{
		Name: "app_key", EnvKey: kg.EnvKey, Fix: kg.Command, Severity: StatusFail,
		Detail: fmt.Sprintf("%s is empty, so encryption, signed URLs, and sessions won't work until it's set.", kg.EnvKey),
	}
	return checkEnvKeySet(envfile.Reader(envPath, "dotenv"), spec), true
}

// checkSQLiteDatabase fails when the env file selects the sqlite driver but the
// database file is missing or empty. Without it the app 500s on the first query
// ("no such table"), which migrate:status can't surface — it errors when the
// migrations table is absent and the doctor degrades that to "unknown". Skipped
// unless DB_CONNECTION is sqlite; an in-memory database has no file to check.
func checkSQLiteDatabase(path, envPath string, fw *config.Framework) (Check, bool) {
	if !strings.EqualFold(strings.TrimSpace(envfile.ReadKey(envPath, "DB_CONNECTION")), "sqlite") {
		return Check{}, false
	}
	dbFile := strings.TrimSpace(envfile.ReadKey(envPath, "DB_DATABASE"))
	if dbFile == ":memory:" {
		return Check{}, false
	}
	if dbFile == "" {
		dbFile = filepath.Join("database", "database.sqlite") // Laravel default
	}
	abs := dbFile
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(path, dbFile)
	}
	// Only offer the migrate button when the framework actually has that command,
	// so the CLI/TUI don't print a fix that maps to nothing.
	fix := ""
	if frameworkHasCommand(fw, sqliteFixCommand) {
		fix = sqliteFixCommand
	}
	info, err := os.Stat(abs)
	switch {
	case err != nil && os.IsNotExist(err):
		return Check{Name: "sqlite_database", Status: StatusFail, Fix: fix,
			Detail: fmt.Sprintf("SQLite database file is missing at %s — create it and run migrations.", dbFile)}, true
	case err != nil:
		return Check{}, false // can't stat for some other reason; don't guess
	case info.Size() == 0:
		return Check{Name: "sqlite_database", Status: StatusFail, Fix: fix,
			Detail: fmt.Sprintf("SQLite database at %s is empty — run migrations to create the schema.", dbFile)}, true
	}
	return Check{Name: "sqlite_database", Status: StatusOK}, true
}

// sqliteFixCommand names the framework command the doctor offers to populate an
// empty or missing SQLite database. Laravel's "migrate" creates the schema (and,
// on current versions, the file); frameworks without a matching command get no fix.
const sqliteFixCommand = "migrate"

// frameworkHasCommand reports whether fw declares a command of the given name, so
// a doctor fix only points at something the site can actually run.
func frameworkHasCommand(fw *config.Framework, name string) bool {
	if fw == nil {
		return false
	}
	for _, c := range fw.Commands {
		if c.Name == name {
			return true
		}
	}
	return false
}

// checkEnvKeySet warns when key is empty in the env file. A check that considers
// the key mandatory escalates with severity: fail.
func checkEnvKeySet(read func(string) string, spec config.DoctorCheck) Check {
	if strings.TrimSpace(read(spec.EnvKey)) == "" {
		detail := spec.Detail
		if detail == "" {
			detail = fmt.Sprintf("%s is empty.", spec.EnvKey)
		}
		return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusWarn), Detail: detail, Fix: spec.Fix}
	}
	return Check{Name: spec.Name, Status: StatusOK}
}

// checkEnvDrift warns when .env.example declares keys the project's .env is
// missing — the classic "pulled main, app breaks on a new env var" trap. Only
// key names are surfaced, never values, so it's safe to return over the wire.
// Skipped (ok=false) when there's no .env.example to compare against.
func checkEnvDrift(path, envPath, examplePath string) (Check, bool) {
	if _, err := os.Stat(examplePath); err != nil {
		return Check{}, false
	}
	missing, ok := missingExampleKeys(examplePath, envPath)
	if !ok {
		return Check{}, false
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

// missingExampleKeys returns the example keys absent from the env file, in
// example order. ok is false only when the example itself can't be read; a
// missing env file yields every example key as missing. Shared by the env_drift
// check and ProposeEnvMerge so both see the same key set.
func missingExampleKeys(examplePath, envPath string) (missing []string, ok bool) {
	exampleKeys, err := envfile.ReadKeys(examplePath)
	if err != nil {
		return nil, false
	}
	have := map[string]bool{}
	if envKeys, err := envfile.ReadKeys(envPath); err == nil {
		for _, k := range envKeys {
			have[k] = true
		}
	}
	for _, k := range exampleKeys {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	return missing, true
}

// EnvMergeProposal carries the raw example and env file contents plus the
// missing keys, classified the same way the env_drift check splits them:
// Required keys the app reads with no default, Optional keys code has a fallback
// for. A caller pairs this with envfile.MergeMissing to build the proposed .env.
type EnvMergeProposal struct {
	EnvFile        string
	ExampleContent string
	EnvContent     string
	Required       []string
	Optional       []string
}

// ProposeEnvMerge resolves the env and example files for the framework rooted at
// path and returns its classified missing keys. ok is false when the framework
// has no dotenv example to compare against (php-const frameworks, or no example
// file on disk), so the caller can skip offering an insert.
func ProposeEnvMerge(path string, fw *config.Framework) (EnvMergeProposal, bool) {
	envFile, format, exampleFile := envSetup(fw, path)
	if format != "dotenv" {
		return EnvMergeProposal{}, false
	}
	examplePath := filepath.Join(path, exampleFile)
	exampleContent, err := os.ReadFile(examplePath)
	if err != nil {
		return EnvMergeProposal{}, false
	}
	envPath := filepath.Join(path, envFile)
	missing, ok := missingExampleKeys(examplePath, envPath)
	if !ok {
		return EnvMergeProposal{}, false
	}
	required, optional := classifyMissingEnvKeys(path, missing)
	envContent, _ := os.ReadFile(envPath) // absent env => empty, MergeMissing handles it
	return EnvMergeProposal{
		EnvFile:        envFile,
		ExampleContent: string(exampleContent),
		EnvContent:     string(envContent),
		Required:       required,
		Optional:       optional,
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

// triggeredStatus returns the status a triggered check reports, honouring a
// per-check Severity override and falling back to def.
func triggeredStatus(spec config.DoctorCheck, def string) string {
	switch strings.ToLower(spec.Severity) {
	case StatusWarn, StatusFail:
		return strings.ToLower(spec.Severity)
	default:
		return def
	}
}

// valueMatches compares an env value to an expected value, treating true/false
// truthily (so APP_DEBUG=1/on/yes all match "true") and everything else as a
// case-insensitive string equality.
func valueMatches(actual, expected string) bool {
	a := strings.ToLower(strings.TrimSpace(actual))
	switch strings.ToLower(strings.TrimSpace(expected)) {
	case "true":
		return a == "true" || a == "1" || a == "on" || a == "yes"
	case "false":
		return a == "false" || a == "0" || a == "off" || a == "no" || a == ""
	default:
		return a == strings.ToLower(strings.TrimSpace(expected))
	}
}

// checkEnvCombo warns (the production footgun pattern) when every key in When
// matches and every key in WarnIf matches — e.g. APP_ENV=production with
// APP_DEBUG on. Any mismatch passes quietly.
func checkEnvCombo(read func(string) string, spec config.DoctorCheck) Check {
	for k, v := range spec.When {
		if !valueMatches(read(k), v) {
			return Check{Name: spec.Name, Status: StatusOK}
		}
	}
	for k, v := range spec.WarnIf {
		if !valueMatches(read(k), v) {
			return Check{Name: spec.Name, Status: StatusOK}
		}
	}
	return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusWarn), Detail: spec.Detail, Fix: spec.Fix}
}

// checkSymlink warns when Link isn't a symlink while Target and RequiresDir both
// exist (Laravel's public/storage link). Skipped when the target/dir is absent,
// where the link is irrelevant.
func checkSymlink(path string, spec config.DoctorCheck) (Check, bool) {
	if fi, err := os.Lstat(filepath.Join(path, spec.Link)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return Check{Name: spec.Name, Status: StatusOK}, true
	}
	if info, err := os.Stat(filepath.Join(path, spec.Target)); err != nil || !info.IsDir() {
		return Check{}, false
	}
	if spec.RequiresDir != "" {
		if info, err := os.Stat(filepath.Join(path, spec.RequiresDir)); err != nil || !info.IsDir() {
			return Check{}, false
		}
	}
	return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusWarn), Detail: spec.Detail, Fix: spec.Fix}, true
}

// checkCommand execs a console command in the site's container and fails when
// the output contains FailIfOutputContains (pending migrations). When the
// command can't run it degrades to "unknown" if UnknownOnError is set, so a
// wedged app never turns the whole panel into an error.
func checkCommand(ctx context.Context, path string, spec config.DoctorCheck) Check {
	timeout := commandTimeout
	if spec.TimeoutSeconds > 0 {
		timeout = time.Duration(spec.TimeoutSeconds) * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, exit, err := runCapture(cctx, path, spec.Command)
	if err != nil || exit != 0 {
		if spec.UnknownOnError {
			return Check{Name: spec.Name, Status: StatusUnknown, Detail: "Couldn't run the check, the app may be down or a dependency unreachable."}
		}
		return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusFail), Detail: spec.Detail, Fix: spec.Fix}
	}
	if spec.FailIfOutputContains != "" && strings.Contains(out, spec.FailIfOutputContains) {
		return Check{Name: spec.Name, Status: triggeredStatus(spec, StatusFail), Detail: spec.Detail, Fix: spec.Fix}
	}
	return Check{Name: spec.Name, Status: StatusOK}
}

// maxDoctorConcurrency bounds how many checks (mostly container execs) run at
// once, so the panel's latency is capped near a single timeout without spawning
// an unbounded number of concurrent podman execs.
const maxDoctorConcurrency = 6

// runChecksConcurrently runs each task in its own goroutine, bounded by
// maxDoctorConcurrency, and returns the checks that fired in the original task
// order, so independent container-exec checks no longer add up their timeouts.
func runChecksConcurrently(tasks []func() (Check, bool)) []Check {
	results := make([]struct {
		c  Check
		ok bool
	}, len(tasks))
	sem := make(chan struct{}, maxDoctorConcurrency)
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		go func(i int, t func() (Check, bool)) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i].c, results[i].ok = t()
		}(i, t)
	}
	wg.Wait()
	var out []Check
	for _, r := range results {
		if r.ok {
			out = append(out, r.c)
		}
	}
	return out
}

// dependencyCheckTasks builds the universal package-manager checks (composer and
// node dependency install + lock state, plus their security audits) as deferred
// tasks. Each is skipped when its manifest is absent, and the composer audit is
// skipped when vendor/ is missing (checkComposerDeps already flags that, and the
// audit can only degrade to "unknown" without installed packages).
func dependencyCheckTasks(ctx context.Context, path string, fw *config.Framework) []func() (Check, bool) {
	var tasks []func() (Check, bool)
	if fileExists(filepath.Join(path, "composer.json")) && !composerDisabled(fw) {
		tasks = append(tasks, func() (Check, bool) { return checkComposerDeps(ctx, path), true })
		if dirExists(filepath.Join(path, "vendor")) {
			tasks = append(tasks, func() (Check, bool) { return checkComposerAudit(ctx, path), true })
		}
	}
	if fileExists(filepath.Join(path, "package.json")) {
		tasks = append(tasks, func() (Check, bool) { return checkNodeDeps(path), true })
		tasks = append(tasks, func() (Check, bool) { return checkNodeAudit(ctx, path), true })
	}
	return tasks
}

// composerDisabled reports whether the framework explicitly opts out of composer
// handling (composer: false in the store).
func composerDisabled(fw *config.Framework) bool {
	return fw != nil && strings.EqualFold(fw.Composer, "false")
}

// checkComposerDeps warns when composer dependencies aren't installed or the
// lock file has drifted from composer.json. Degrades to "unknown" when composer
// can't run.
func checkComposerDeps(ctx context.Context, path string) Check {
	if !dirExists(filepath.Join(path, "vendor")) {
		return Check{Name: "composer_deps", Status: StatusWarn, Detail: "Composer dependencies aren't installed, run composer install.", Fix: FixComposerInstall}
	}
	if !fileExists(filepath.Join(path, "composer.lock")) {
		return Check{Name: "composer_deps", Status: StatusWarn, Detail: "No composer.lock is committed, run composer install to create one.", Fix: FixComposerInstall}
	}
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, exit, err := runCapture(cctx, path, "composer validate --no-check-all --no-check-publish")
	if err != nil || exit < 0 {
		return Check{Name: "composer_deps", Status: StatusUnknown, Detail: "Couldn't validate composer state."}
	}
	if composerLockStale(out) {
		return Check{Name: "composer_deps", Status: StatusWarn, Detail: "composer.lock is out of date with composer.json, run composer update.", Fix: FixComposerUpdate}
	}
	return Check{Name: "composer_deps", Status: StatusOK}
}

// composerLockStale reports whether `composer validate` flagged the lock file as
// out of date with composer.json.
func composerLockStale(output string) bool {
	o := strings.ToLower(output)
	return strings.Contains(o, "lock file is not up to date") || strings.Contains(o, "lock file is out of date")
}

// checkComposerAudit warns when `composer audit` reports advisories against the
// installed packages. Network-dependent, so it degrades to "unknown" offline.
func checkComposerAudit(ctx context.Context, path string) Check {
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, exit, err := runCapture(cctx, path, "composer audit --format=json --no-interaction")
	if err != nil || exit < 0 {
		return Check{Name: "composer_audit", Status: StatusUnknown, Detail: "Couldn't run composer audit, packages may not be installed or the network is unreachable."}
	}
	n := parseComposerAudit(out)
	if n < 0 {
		return Check{Name: "composer_audit", Status: StatusUnknown, Detail: "Couldn't read composer audit output."}
	}
	if n > 0 {
		return Check{Name: "composer_audit", Status: StatusWarn, Detail: fmt.Sprintf("%d known security advisor%s in composer packages, run composer update.", n, plural(n, "y", "ies")), Fix: FixComposerUpdate}
	}
	return Check{Name: "composer_audit", Status: StatusOK}
}

// parseComposerAudit returns the total advisory count in `composer audit
// --format=json` output, or -1 when the JSON can't be read. The advisories field
// is an object keyed by package when present but an empty array when there are
// none, so both shapes are handled.
func parseComposerAudit(output string) int {
	start := strings.IndexByte(output, '{')
	if start < 0 {
		return -1
	}
	var parsed struct {
		Advisories json.RawMessage `json:"advisories"`
	}
	// Decode only the first JSON object: composer can print warning/deprecation
	// lines after the payload, which json.Unmarshal rejects as trailing data and
	// would otherwise degrade a real advisory count to "unknown".
	if err := json.NewDecoder(strings.NewReader(output[start:])).Decode(&parsed); err != nil {
		return -1
	}
	if len(parsed.Advisories) == 0 {
		return 0
	}
	var byPackage map[string][]json.RawMessage
	if json.Unmarshal(parsed.Advisories, &byPackage) == nil {
		total := 0
		for _, advs := range byPackage {
			total += len(advs)
		}
		return total
	}
	var empty []json.RawMessage
	if json.Unmarshal(parsed.Advisories, &empty) == nil {
		return len(empty)
	}
	return -1
}

// checkNodeDeps warns when node dependencies aren't installed or no lockfile is
// committed (npm, pnpm, yarn, or bun).
func checkNodeDeps(path string) Check {
	if !dirExists(filepath.Join(path, "node_modules")) {
		return Check{Name: "node_deps", Status: StatusWarn, Detail: "Node dependencies aren't installed, run npm install.", Fix: FixNpmInstall}
	}
	for _, lock := range []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"} {
		if fileExists(filepath.Join(path, lock)) {
			return Check{Name: "node_deps", Status: StatusOK}
		}
	}
	return Check{Name: "node_deps", Status: StatusWarn, Detail: "No lockfile is committed, installs won't be reproducible."}
}

// checkNodeAudit warns when `npm audit` reports vulnerabilities. Network- and
// lock-dependent, so it degrades to "unknown" when it can't run.
func checkNodeAudit(ctx context.Context, path string) Check {
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, exit, err := runCapture(cctx, path, "npm audit --json")
	if err != nil || exit < 0 {
		return Check{Name: "node_audit", Status: StatusUnknown, Detail: "Couldn't run npm audit, dependencies may not be installed or the network is unreachable."}
	}
	n := parseNpmAudit(out)
	if n < 0 {
		return Check{Name: "node_audit", Status: StatusUnknown, Detail: "Couldn't read npm audit output."}
	}
	if n > 0 {
		return Check{Name: "node_audit", Status: StatusWarn, Detail: fmt.Sprintf("%d known vulnerabilit%s in node packages, run npm audit fix.", n, plural(n, "y", "ies")), Fix: FixNpmAuditFix}
	}
	return Check{Name: "node_audit", Status: StatusOK}
}

// parseNpmAudit returns the total vulnerability count in `npm audit --json`
// output, or -1 when the audit couldn't actually run or its JSON can't be read.
func parseNpmAudit(output string) int {
	start := strings.IndexByte(output, '{')
	if start < 0 {
		return -1
	}
	var parsed struct {
		Error    json.RawMessage `json:"error"`
		Metadata struct {
			Vulnerabilities struct {
				Total int `json:"total"`
			} `json:"vulnerabilities"`
		} `json:"metadata"`
	}
	// Decode only the first JSON object: npm can print warning lines after the
	// payload, which json.Unmarshal rejects as trailing data.
	if err := json.NewDecoder(strings.NewReader(output[start:])).Decode(&parsed); err != nil {
		return -1
	}
	// On a pnpm/yarn/bun project (no package-lock.json) npm audit errors with
	// {"error":{"code":"ENOLOCK",...}} and no metadata block. Total would default
	// to 0, falsely reporting a clean audit, so report "unknown" instead.
	if len(parsed.Error) > 0 {
		return -1
	}
	return parsed.Metadata.Vulnerabilities.Total
}

// checkPHPVersion warns when the project's resolved PHP version falls outside the
// framework's supported range. Skipped when fw declares no range.
func checkPHPVersion(path string, fw *config.Framework) (Check, bool) {
	if fw == nil || (fw.PHP.Min == "" && fw.PHP.Max == "") {
		return Check{}, false
	}
	v, err := phpkg.DetectVersion(path)
	if err != nil || v == "" {
		return Check{}, false
	}
	if fw.PHP.Min != "" && phpkg.CompareMajorMinor(v, fw.PHP.Min) < 0 {
		return Check{Name: "php_version", Status: StatusWarn, Detail: fmt.Sprintf("PHP %s is below %s's minimum %s.", v, fw.Label, fw.PHP.Min)}, true
	}
	if fw.PHP.Max != "" && phpkg.CompareMajorMinor(v, fw.PHP.Max) > 0 {
		return Check{Name: "php_version", Status: StatusWarn, Detail: fmt.Sprintf("PHP %s is above %s's tested maximum %s.", v, fw.Label, fw.PHP.Max)}, true
	}
	return Check{Name: "php_version", Status: StatusOK}, true
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// runCapture runs a shell command in cwd with lerd's bin shims on PATH (so php,
// composer, and npm resolve to the container shims under launchd's restricted
// PATH), mirroring the command runner. Returns combined output and the exit
// code; a non-ExitError (couldn't even start) comes back as exit -1.
func runCapture(ctx context.Context, cwd, command string) (string, int, error) {
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
