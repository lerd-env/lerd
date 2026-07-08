// Package shims is the single owner of the client-tool host shims that lerd
// services expose (mysqldump, pg_dump, psql…): the tri-state install decisions,
// host-conflict detection, script generation, and the reconcile that brings the
// shim dir in line with the installed services.
//
// Scope is deliberately just these service client tools. The php/composer/node
// shims are a different category (always-on or a single global toggle, exec on
// the host or into the FPM container) and stay owned by the installer's
// addShellShims and the shim_sync mirror; they are intentionally not managed
// here.
package shims

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
	"gopkg.in/yaml.v3"
)

// marker tags every generated shim so the reconcile only ever touches its own
// files, never a user binary or another shim category.
const marker = "lerd-managed service client shim"

// shimNamePattern allowlists a store-declared client-tool name: it becomes a
// filename and a token in the generated script, so a path separator or shell
// char could escape BinDir or inject a line. Real tool names all match.
var shimNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// validShimName reports whether a client-tool or candidate-binary name is safe
// to use as a shim filename and as a token in the generated shell script.
func validShimName(name string) bool {
	return shimNamePattern.MatchString(name)
}

// reservedShimNames are the host-shim filenames the installer owns (addShellShims
// and the fnm node mirror). They share BinDir with the client shims but carry no
// marker, so a store-declared client tool must never claim one, else a preset
// could repoint the host php or composer at a database client container.
var reservedShimNames = map[string]bool{
	"php": true, "composer": true, "composer.phar": true, "laravel": true,
	"node": true, "npm": true, "npx": true, "fnm": true,
}

// isReservedShimName reports whether name is an installer-owned shim the client
// reconcile must not generate over.
func isReservedShimName(name string) bool {
	return reservedShimNames[name]
}

// validBinaries keeps only the candidate binary names safe to pass to
// client-exec, dropping any a malformed or hostile store entry slipped in.
func validBinaries(bins []string) []string {
	out := bins[:0:0]
	for _, b := range bins {
		if validShimName(b) {
			out = append(out, b)
		}
	}
	return out
}

// Target records which service container a tool shim execs into and the
// candidate binaries to resolve there (mariadb images ship mariadb-dump in
// place of mysqldump, so the shim tries each in order).
type Target struct {
	Service  string
	Binaries []string
}

// Info is the per-tool shim state the CLI and web UI render: which service
// backs it, whether the host already has the tool (so callers can warn before
// shadowing it), whether a decision has been recorded, and whether it's on.
// Owner is the service that actually backs the shim when several same-family
// services expose the same tool; a service whose name differs from Owner does
// not manage the shim and its toggle is shown disabled.
type Info struct {
	Tool    string `json:"tool"`
	Service string `json:"service,omitempty"`
	Owner   string `json:"owner,omitempty"`
	HostHas bool   `json:"host_has"`
	Enabled bool   `json:"enabled"`
	Decided bool   `json:"decided"`
}

// Prompter resolves a host-conflict when the host already has a tool. It
// returns whether to enable the shim and whether a decision was actually made.
// A nil Prompter leaves the conflict undecided so a later interactive run or the
// web UI can resolve it.
type Prompter func(tool string) (enable, decided bool)

// script renders the host shim: a trivial pass-through into
// `lerd client-exec <tool>`, so args, stdin, stdout and exit code all behave
// exactly like the native binary an IDE would call.
func script(lerdBin, tool string) string {
	return "#!/bin/sh\n# " + marker + "\nexec " + lerdBin + " client-exec " + tool + " \"$@\"\n"
}

// Targets maps every client tool exposed by an installed service to the
// container and candidate binaries that back it. The first installed service to
// declare a given tool name wins, so a mysql and a mariadb install both exposing
// `mysql` resolve deterministically.
func Targets() map[string]Target {
	out := map[string]Target{}
	for _, name := range installedServiceNames() {
		svc, err := loadServiceDef(name)
		if err != nil || svc == nil {
			continue
		}
		for _, cs := range svc.ClientShims {
			if !validShimName(cs.Name) || isReservedShimName(cs.Name) {
				continue
			}
			if _, exists := out[cs.Name]; exists {
				continue
			}
			bins := cs.Binaries
			if len(bins) == 0 {
				bins = []string{cs.Name}
			} else {
				bins = validBinaries(bins)
				if len(bins) == 0 {
					continue
				}
			}
			out[cs.Name] = Target{Service: name, Binaries: bins}
		}
	}
	return out
}

// installedServiceNames lists every service whose container unit is installed,
// covering canonical default presets and installed add-ons / alternate versions.
func installedServiceNames() []string {
	var names []string
	for _, n := range config.DefaultPresetNames() {
		if services.Mgr != nil && services.Mgr.ContainerUnitInstalled("lerd-"+n) {
			names = append(names, n)
		}
	}
	customs, _ := config.ListCustomServices()
	for _, c := range customs {
		if services.Mgr != nil && services.Mgr.ContainerUnitInstalled("lerd-"+c.Name) {
			names = append(names, c.Name)
		}
	}
	return names
}

// loadServiceDef resolves a service's definition whether it is a canonical
// default preset (mysql, postgres) resolved from the embedded YAML or an
// installed add-on / alternate version stored as a custom service.
func loadServiceDef(name string) (*config.CustomService, error) {
	if config.IsDefaultPreset(name) {
		p, err := config.LoadPreset(name)
		if err != nil {
			return nil, err
		}
		return p.Resolve("")
	}
	return config.LoadCustomService(name)
}

// ServiceShims returns the client-tool shim state for one installed service,
// one entry per tool it declares. Owner is filled from the global targets so a
// service that only shares a same-family tool (not its owner) is marked. Empty
// when the service exposes none.
func ServiceShims(name string) []Info {
	svc, err := loadServiceDef(name)
	if err != nil || svc == nil {
		return nil
	}
	targets := Targets()
	var out []Info
	for _, cs := range svc.ClientShims {
		if cs.Name == "" {
			continue
		}
		enabled, decided := decision(cs.Name)
		out = append(out, Info{
			Tool:    cs.Name,
			Service: name,
			Owner:   targets[cs.Name].Service,
			HostHas: hostHasTool(cs.Name),
			Enabled: enabled,
			Decided: decided,
		})
	}
	return out
}

// ToolOwner returns the installed service that backs the tool's shim, or "" when
// no installed service exposes it. Used to guard toggles so a shared shim is
// only managed from its owner.
func ToolOwner(tool string) string {
	return Targets()[tool].Service
}

// ResolveTarget picks the container and binaries to run a tool in. When prefer
// names an installed service that exposes the tool, it wins (site-aware
// routing: a dump run from a project targets that project's own database
// service). Otherwise it falls back to the global owner. The bool is false only
// when no installed service exposes the tool at all.
func ResolveTarget(tool, prefer string) (Target, bool) {
	if prefer != "" {
		if t, ok := serviceTarget(prefer, tool); ok {
			return t, true
		}
	}
	t, ok := Targets()[tool]
	return t, ok
}

// serviceTarget returns the Target for a tool as declared by a specific
// installed service, or false when that service does not expose the tool.
func serviceTarget(service, tool string) (Target, bool) {
	svc, err := loadServiceDef(service)
	if err != nil || svc == nil {
		return Target{}, false
	}
	for _, cs := range svc.ClientShims {
		if cs.Name == tool {
			bins := cs.Binaries
			if len(bins) == 0 {
				bins = []string{tool}
			}
			return Target{Service: service, Binaries: bins}, true
		}
	}
	return Target{}, false
}

// List returns every exposed client tool with its state, sorted by tool name,
// for the `lerd shims` listing.
func List() []Info {
	targets := Targets()
	tools := make([]string, 0, len(targets))
	for t := range targets {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	out := make([]Info, 0, len(tools))
	for _, t := range tools {
		enabled, decided := decision(t)
		out = append(out, Info{
			Tool:    t,
			Service: targets[t].Service,
			HostHas: hostHasTool(t),
			Enabled: enabled,
			Decided: decided,
		})
	}
	return out
}

// Set is the single entry point for an explicit shim decision, shared by the
// CLI (`lerd shims add/remove`) and the web UI toggle. It validates that an
// installed service actually exposes the tool, records the decision, and
// reconciles so the change takes effect at once. It never prompts.
func Set(tool string, enabled bool) error {
	if _, ok := Targets()[tool]; !ok {
		return fmt.Errorf("no installed service exposes the %q client tool", tool)
	}
	if err := setDecision(tool, enabled); err != nil {
		return err
	}
	return Reconcile(nil)
}

// Reconcile brings the host shim dir in line with the tools the installed
// services expose and the recorded decisions. prompt resolves a host-conflict
// on first sight; pass nil on non-interactive paths (a removal, an update
// reconcile) to leave conflicts undecided. Shims for tools no longer exposed by
// any installed service are pruned.
func Reconcile(prompt Prompter) error {
	lerdBin, err := os.Executable()
	if err != nil {
		return err
	}
	binDir := config.BinDir()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	targets := Targets()
	for tool := range targets {
		enabled, _ := decide(tool, prompt)
		shimPath := filepath.Join(binDir, tool)
		if enabled {
			if canWriteShim(shimPath) {
				_ = os.WriteFile(shimPath, []byte(script(lerdBin, tool)), 0755)
			}
		} else {
			removeIfShim(shimPath)
		}
	}
	pruneOrphans(targets)
	return nil
}

// decide resolves whether a tool's shim should be installed. A recorded decision
// wins. Otherwise, when the host lacks the tool there is no conflict so the shim
// is enabled automatically; when the host already has it, prompt decides (and
// the answer is recorded). A nil prompt leaves the conflict undecided.
func decide(tool string, prompt Prompter) (enabled, decided bool) {
	if e, d := decision(tool); d {
		return e, true
	}
	if !hostHasTool(tool) {
		_ = setDecision(tool, true)
		return true, true
	}
	if prompt == nil {
		return false, false
	}
	e, d := prompt(tool)
	if d {
		_ = setDecision(tool, e)
	}
	return e, d
}

// hostHasTool reports whether the user already has the named tool on their PATH,
// excluding lerd's own shim dir so a previously-installed lerd shim is never
// mistaken for a real host binary.
func hostHasTool(tool string) bool {
	binDir := config.BinDir()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || dir == binDir {
			continue
		}
		cand := filepath.Join(dir, tool)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return true
		}
	}
	return false
}

// canWriteShim reports whether the reconcile may write path: only when it is
// absent or already one of lerd's client shims. An existing non-shim file (the
// installer's php/composer, or a user binary sharing a tool name) is left intact,
// mirroring removeIfShim so the write and remove sides guard the same set.
func canWriteShim(path string) bool {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return true
	}
	return isShimFile(path)
}

// removeIfShim deletes path only when it is one of lerd's client shims, so the
// reconcile never removes a user's own binary of the same name.
func removeIfShim(path string) {
	if isShimFile(path) {
		_ = os.Remove(path)
	}
}

// pruneOrphans removes generated shims whose tool is no longer exposed by any
// installed service and forgets the stale decision so a later reinstall
// re-decides fresh.
func pruneOrphans(targets map[string]Target) {
	binDir := config.BinDir()
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		tool := e.Name()
		if _, ok := targets[tool]; ok {
			continue
		}
		path := filepath.Join(binDir, tool)
		if isShimFile(path) {
			_ = os.Remove(path)
			_ = forgetDecision(tool)
		}
	}
}

// isShimFile reports whether path is a lerd-managed client shim, matched by the
// marker comment its generator writes.
func isShimFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), marker)
}

// ── decision storage ────────────────────────────────────────────────────────
//
// Decisions are tri-state: a tool absent from the file is undecided (the
// reconcile still has to choose), present-true is an installed shim,
// present-false is a shim the user declined. The set-based service flag files
// can't express the declined state, so this uses its own map file.

func decisionsFile() string {
	return filepath.Join(config.DataDir(), "client-shim-decisions.yaml")
}

// decision returns whether the host shim for the tool is enabled and whether any
// decision has been recorded.
func decision(tool string) (enabled, decided bool) {
	m, err := loadDecisions()
	if err != nil {
		return false, false
	}
	v, ok := m[tool]
	return v, ok
}

func setDecision(tool string, enabled bool) error {
	m, err := loadDecisions()
	if err != nil {
		m = map[string]bool{}
	}
	m[tool] = enabled
	return saveDecisions(m)
}

func forgetDecision(tool string) error {
	m, err := loadDecisions()
	if err != nil || len(m) == 0 {
		return nil
	}
	if _, ok := m[tool]; !ok {
		return nil
	}
	delete(m, tool)
	return saveDecisions(m)
}

func loadDecisions() (map[string]bool, error) {
	data, err := os.ReadFile(decisionsFile())
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func saveDecisions(m map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(decisionsFile()), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(decisionsFile(), data, 0644)
}
