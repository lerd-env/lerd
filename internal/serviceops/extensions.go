package serviceops

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Extension is an engine's declared extension, aliased so callers work in one
// package without the store type leaking a second definition.
type Extension = config.Extension

// DeclaredExtensions returns what an engine's image can create, preferring the
// preset the service was installed from so an engine installed before the field
// existed still gets it, the same resolution IntrospectCommand uses.
func DeclaredExtensions(service string) []Extension {
	presetName := service
	if custom, err := config.LoadCustomService(service); err == nil {
		if len(custom.Extensions) > 0 {
			return custom.Extensions
		}
		if custom.Preset != "" {
			presetName = custom.Preset
		}
	}
	if p, err := config.LoadPreset(presetName); err == nil {
		return p.Extensions
	}
	return nil
}

// alwaysExtensions returns the declared extensions created up front rather than
// when a dump reaches for one of their types.
func alwaysExtensions(exts []Extension) []Extension {
	var out []Extension
	for _, e := range exts {
		if e.Always {
			out = append(out, e)
		}
	}
	return out
}

// ensureAlwaysExtensions creates an engine's up-front extensions in a database.
// Every path that makes a database calls it, so one that gets dropped and
// recreated, by a fresh import or a snapshot restore, comes back whole.
func ensureAlwaysExtensions(service, database string, exts []Extension) error {
	for _, e := range alwaysExtensions(exts) {
		if err := createExtensionFn(service, database, e.Name); err != nil {
			return fmt.Errorf("extension %s in %s: %w", e.Name, database, err)
		}
	}
	return nil
}

// EnsureExtensions applies an engine's up-front extensions to a database it has
// just created.
func EnsureExtensions(service, database string) error {
	return ensureAlwaysExtensions(service, database, DeclaredExtensions(service))
}

// typePattern matches a declared type where a dump would name one: as a column
// type, schema qualified or not, and never as part of a longer identifier.
var (
	patternCache sync.Map
	typeWord     = regexp.MustCompile(`\W`)
)

func typePattern(name string) *regexp.Regexp {
	if re, ok := patternCache.Load(name); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(?i)(^|[^\w.])(\w+\.)?` + regexp.QuoteMeta(name) + `\b`)
	patternCache.Store(name, re)
	return re
}

// ddlLine matches where a type may legitimately be named: the head of a CREATE
// or ALTER, or an indented continuation of one, which is how pg_dump and
// mysqldump both write column definitions. It deliberately excludes INSERT and
// the rest, so the word inside a row value is never read as a type.
var ddlLine = regexp.MustCompile(`(?i)^(\s+|\s*(CREATE|ALTER)\b)`)

// extensionForLine returns the extension a dump line reaches for, or nil.
func extensionForLine(exts []Extension, line string) *Extension {
	if len(exts) == 0 || !ddlLine.MatchString(line) {
		return nil
	}
	for i := range exts {
		for _, t := range exts[i].Types {
			if m := typePattern(t).FindStringIndex(line); m != nil && !typeFollowedByWord(line, m[1]) {
				return &exts[i]
			}
		}
	}
	return nil
}

// typeFollowedByWord reports whether the match runs into more identifier, so
// "vectorized" never counts as a reference to "vector".
func typeFollowedByWord(line string, end int) bool {
	if end >= len(line) {
		return false
	}
	return !typeWord.MatchString(string(line[end]))
}

// CreateExtension creates one extension in a database, idempotently, so a dump
// that carries its own CREATE EXTENSION and lerd's own call cannot collide.
func CreateExtension(service, database, name string) error {
	if err := ValidateDatabaseName(database); err != nil {
		return err
	}
	if !validExtensionName(name) {
		return fmt.Errorf("invalid extension name %q", name)
	}
	sql := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %q", name)
	out, err := containerExec("lerd-"+service,
		"psql -U postgres -v ON_ERROR_STOP=1 -d "+podman.ShellQuote(database)+" -c "+podman.ShellQuote(sql),
		[]string{"PGPASSWORD=lerd"}, nil, introspectTimeout)
	if err != nil {
		return fmt.Errorf("creating extension %s in %s: %w\n%s", name, database, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstalledExtensions returns the extensions already present in a database.
func InstalledExtensions(service, database string) ([]string, error) {
	if err := ValidateDatabaseName(database); err != nil {
		return nil, err
	}
	out, err := containerExec("lerd-"+service,
		"psql -U postgres -At -d "+podman.ShellQuote(database)+" -c "+
			podman.ShellQuote("SELECT extname FROM pg_extension ORDER BY extname"),
		[]string{"PGPASSWORD=lerd"}, nil, introspectTimeout)
	if err != nil {
		return nil, fmt.Errorf("listing extensions in %s: %w\n%s", database, err, strings.TrimSpace(string(out)))
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

var extensionName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validExtensionName(name string) bool { return extensionName.MatchString(name) }
