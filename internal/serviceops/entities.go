package serviceops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// entityActionTimeout caps a mutating entity action. Creates are quick but a
// drop of a large database can take minutes, so this is generous while still
// unsticking a wedged engine.
const entityActionTimeout = 5 * time.Minute

// EntityRow is one listed entity: its name, then the declared columns' values
// in declaration order, as the list command printed them.
type EntityRow struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// resolveIntrospect returns a service's introspect block. The preset is
// preferred over the stored definition so a store publish reaches services
// installed before it, with the stored block as the fallback for a genuinely
// user-defined engine.
func resolveIntrospect(service string) *config.Introspect {
	var stored *config.Introspect
	presetName := service
	if custom, err := config.LoadCustomService(service); err == nil {
		stored = custom.Introspect
		if custom.Preset != "" {
			presetName = custom.Preset
		}
	}
	if p, err := config.LoadPreset(presetName); err == nil && p.Introspect != nil {
		return p.Introspect
	}
	return stored
}

// EntityFor resolves the declared entity spec of the given kind for a service.
// Kind "databases" also honours the legacy list_databases field.
func EntityFor(service, kind string) *config.EntitySpec {
	in := resolveIntrospect(service)
	if kind == "databases" {
		return in.DatabasesEntity()
	}
	return in.Entity(kind)
}

// EntityAction returns the declared action, reporting whether the service
// declares it at all, so callers can tell "unsupported" from "empty".
func entityAction(spec *config.EntitySpec, action string) (config.EntityAction, bool) {
	if spec == nil {
		return config.EntityAction{}, false
	}
	act, ok := spec.Actions[action]
	return act, ok
}

// expandEntityCommand substitutes every {{name}} in a declared command with the
// entity name. Validation is the injection guard: the strict name pattern
// excludes every shell and SQL metacharacter, so the validated name is safe to
// splice both as a shell word and inside a quoted identifier.
func expandEntityCommand(command, name string) (string, error) {
	if err := ValidateDatabaseName(name); err != nil {
		return "", err
	}
	// Folded YAML scalars carry a trailing newline; trim so composed pipelines
	// stay one line.
	return strings.ReplaceAll(strings.TrimSpace(command), "{{name}}", name), nil
}

// actionRuntime resolves where a declared action runs: its own image and env
// override, else the entity's, so one entity can list through one client and
// stream archives through another.
func actionRuntime(spec *config.EntitySpec, act config.EntityAction) (image string, env []string) {
	if act.Image != "" {
		return act.Image, act.Env
	}
	return spec.Image, spec.Env
}

// entityCommandArgs builds the podman argv that runs shellCmd for an entity:
// an exec inside the service container, or, when a client image is named, an
// ephemeral run of that image on the lerd network, for services whose own
// image ships no tooling for what they hold. interactive adds -i so a command
// can read its stdin.
func entityCommandArgs(service, image string, env []string, shellCmd string, interactive bool) []string {
	if image == "" {
		args := []string{"exec"}
		if interactive {
			args = append(args, "-i")
		}
		for _, kv := range append(introspectEnv(), env...) {
			args = append(args, "--env", kv)
		}
		return append(args, "lerd-"+service, "sh", "-c", shellCmd)
	}
	args := []string{"run", "--rm"}
	if interactive {
		args = append(args, "-i")
	}
	// --entrypoint sh: client images (mc, rclone) make their tool the
	// entrypoint, which would swallow the shell command as its own arguments.
	args = append(args, "--network", "lerd", "--entrypoint", "sh")
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	return append(args, image, "-c", shellCmd)
}

// runEntityCommand runs one non-streaming declared command and returns its
// combined output.
func runEntityCommand(service string, spec *config.EntitySpec, image string, env []string, shellCmd string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return podman.CmdContext(ctx, entityCommandArgs(service, image, env, shellCmd, false)...).CombinedOutput()
}

// ListEntities runs a spec's list command and parses its tab-separated rows.
func ListEntities(service string, spec *config.EntitySpec) ([]EntityRow, error) {
	if spec == nil || strings.TrimSpace(spec.List) == "" {
		return nil, nil
	}
	out, err := runEntityCommand(service, spec, spec.Image, spec.Env, spec.List, introspectTimeout)
	if err != nil {
		return nil, fmt.Errorf("listing %s in %s: %w\n%s", spec.Kind, service, err, strings.TrimSpace(string(out)))
	}
	return parseEntityRows(out), nil
}

// parseEntityRows turns tab-separated lines into rows: the first field is the
// entity name, the rest are column values kept as printed.
func parseEntityRows(out []byte) []EntityRow {
	var rows []EntityRow
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		row := EntityRow{Name: name}
		for _, f := range fields[1:] {
			row.Values = append(row.Values, strings.TrimSpace(f))
		}
		rows = append(rows, row)
	}
	return rows
}

// EntityExists reports whether the named entity is in the spec's list output.
func EntityExists(service string, spec *config.EntitySpec, name string) (bool, error) {
	rows, err := ListEntities(service, spec)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// RunEntityAction runs a declared non-streaming action against one entity.
func RunEntityAction(service string, spec *config.EntitySpec, action, name string) error {
	act, ok := entityAction(spec, action)
	if !ok {
		return fmt.Errorf("%s does not support %s on %s", service, action, spec.Kind)
	}
	cmd, err := expandEntityCommand(act.Exec, name)
	if err != nil {
		return err
	}
	image, env := actionRuntime(spec, act)
	out, err := runEntityCommand(service, spec, image, env, cmd, entityActionTimeout)
	if err != nil {
		return fmt.Errorf("%s %s in %s: %w\n%s", action, spec.Kind, service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// streamEntityAction runs a declared streaming action with its stdout and
// stdin wired to the caller, capturing stderr for error context.
func streamEntityAction(service string, spec *config.EntitySpec, action, name string, stdout io.Writer, stdin io.Reader) error {
	act, ok := entityAction(spec, action)
	if !ok {
		return fmt.Errorf("%s does not support %s on %s", service, action, spec.Kind)
	}
	shellCmd, err := expandEntityCommand(act.Exec, name)
	if err != nil {
		return err
	}
	image, env := actionRuntime(spec, act)
	ctx, cancel := context.WithTimeout(context.Background(), dumpRestoreTimeout)
	defer cancel()
	cmd := podman.CmdContext(ctx, entityCommandArgs(service, image, env, shellCmd, stdin != nil)...)
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	cmd.Stdin = stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s in %s: %w\n%s", action, spec.Kind, service, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ExportEntity streams the declared export of one entity (a bucket's objects
// as a tar, a database dump) to w.
func ExportEntity(service, kind, name string, w io.Writer) error {
	spec := EntityFor(service, kind)
	if spec == nil {
		return fmt.Errorf("%s declares no %s", service, kind)
	}
	return streamEntityAction(service, spec, "export", name, w, nil)
}

// ImportEntity streams an uploaded archive from r into one entity through the
// declared import. A gzipped upload is unwrapped here, the same way database
// dumps are, so a declared import always reads the plain format it expects and
// both a .tar and a .tar.gz load.
func ImportEntity(service, kind, name string, r io.Reader) error {
	spec := EntityFor(service, kind)
	if spec == nil {
		return fmt.Errorf("%s declares no %s", service, kind)
	}
	src, err := DumpReader(r)
	if err != nil {
		return err
	}
	return streamEntityAction(service, spec, "import", name, io.Discard, src)
}

// SnapshotExportFilename is the download name for a stored snapshot. A snapshot
// downloads in the engine's own dump format, so the extension comes from the
// declared export filename (.archive for mongo, .sql for the SQL engines),
// defaulting to .sql. The snapshot name is sanitized so a bad name can neither
// escape the header nor carry a path.
func SnapshotExportFilename(service, snapshot string) string {
	ext := ".sql"
	act, _ := entityAction(EntityFor(service, "databases"), "export")
	if i := strings.LastIndex(act.Filename, "."); i >= 0 {
		ext = act.Filename[i:]
	}
	clean, err := sanitizeSnapshotName(snapshot)
	if err != nil {
		return "snapshot" + ext
	}
	return clean + ext
}

// EntityExportFilename is the download name for an entity export, from the
// declared filename, with a neutral fallback. The result becomes an output
// path in the CLI and MCP export paths, so a name that fails validation never
// shapes it.
func EntityExportFilename(service, kind, name string) string {
	if err := ValidateDatabaseName(name); err != nil {
		return "export.dump"
	}
	act, _ := entityAction(EntityFor(service, kind), "export")
	if act.Filename != "" {
		if out, err := expandEntityCommand(act.Filename, name); err == nil {
			return out
		}
	}
	return name + ".dump"
}

// DeclaresDatabases reports whether a service holds databases the UI can
// enumerate, read off its own declaration rather than a known family, so a
// store-published engine reaches the Databases tab without a Go change.
func DeclaresDatabases(service string) bool {
	return EntityFor(service, "databases") != nil
}

// ServiceEntities returns the non-database entity specs a service declares.
// The databases kind is excluded because the Databases tab is its surface.
func ServiceEntities(service string) []config.EntitySpec {
	in := resolveIntrospect(service)
	if in == nil {
		return nil
	}
	var out []config.EntitySpec
	for _, e := range in.Entities {
		if e.Kind != "databases" {
			out = append(out, e)
		}
	}
	return out
}

// EntityKinds lists the non-database entity kinds a service declares, in
// declaration order.
func EntityKinds(service string) []string {
	specs := ServiceEntities(service)
	if len(specs) == 0 {
		return nil
	}
	kinds := make([]string, len(specs))
	for i, s := range specs {
		kinds[i] = s.Kind
	}
	return kinds
}

// DatabaseActionDeclared reports whether the engine declares the named action
// on its databases entity, so API layers can gate what they offer.
func DatabaseActionDeclared(service, action string) bool {
	_, ok := entityAction(EntityFor(service, "databases"), action)
	return ok
}

// DatabaseDumpFormat returns the declared dump format of the engine's
// databases entity ("sql" turns on the SQL sanitizer and error tally), or
// empty when none is declared.
func DatabaseDumpFormat(service string) string {
	if spec := EntityFor(service, "databases"); spec != nil {
		return spec.Format
	}
	return ""
}

// EntityExecEnv is the fixed admin-credential env every declared command runs
// with, exported for callers that exec the command themselves.
func EntityExecEnv() []string { return introspectEnv() }

// ExportShellCommand resolves the declared in-container command that dumps one
// database to stdout.
func ExportShellCommand(service, database string) (string, error) {
	act, ok := entityAction(EntityFor(service, "databases"), "export")
	if !ok {
		return "", fmt.Errorf("exporting %s databases is not supported", service)
	}
	return expandEntityCommand(act.Exec, database)
}

// ImportShellCommand resolves the declared in-container command that loads a
// dump from stdin into one database.
func ImportShellCommand(service, database string) (string, error) {
	act, ok := entityAction(EntityFor(service, "databases"), "import")
	if !ok {
		return "", fmt.Errorf("importing into %s databases is not supported", service)
	}
	return expandEntityCommand(act.Exec, database)
}

// CloneDatabase copies the schema and data of src into dst inside the same
// service container, by piping the declared export straight into the declared
// import. dst must already exist.
func CloneDatabase(service, src, dst string) error {
	exportCmd, err := ExportShellCommand(service, src)
	if err != nil {
		return err
	}
	importCmd, err := ImportShellCommand(service, dst)
	if err != nil {
		return err
	}
	shellCmd := "( " + exportCmd + " ) | ( " + importCmd + " )"
	out, err := containerExec("lerd-"+service, shellCmd, introspectEnv(), nil, dumpRestoreTimeout)
	if err != nil {
		return fmt.Errorf("clone %s -> %s: %s", src, dst, strings.TrimSpace(string(out)))
	}
	return nil
}

// entitySnapshotDumpCommand wraps a declared export so its dump crosses podman
// exec gzipped, which is how snapshots are stored.
func entitySnapshotDumpCommand(exportCmd string) string {
	return "( " + strings.TrimSpace(exportCmd) + " ) | gzip -c"
}

// entitySnapshotRestoreCommand feeds a stored gzipped dump into a declared
// import.
func entitySnapshotRestoreCommand(importCmd string) string {
	return "gunzip -c | ( " + strings.TrimSpace(importCmd) + " )"
}
