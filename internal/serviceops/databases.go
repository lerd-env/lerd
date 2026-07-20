package serviceops

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// introspectTimeout caps a single list-databases exec so a wedged engine can't
// block a UI request forever.
const introspectTimeout = 15 * time.Second

// DatabaseInfo is one user database inside an engine, with its on-disk size.
type DatabaseInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// introspectEnv supplies the fixed admin credentials lerd sets on every built-in
// DB container, passed via the exec env so no password lands in argv. The engine
// picks whichever variable applies and ignores the rest.
func introspectEnv() []string { return []string{"MYSQL_PWD=lerd", "PGPASSWORD=lerd"} }

// IntrospectCommand resolves an engine's list-databases query, preferring the
// preset it was installed from so an engine installed before the introspect
// field existed still gets it from the current preset. Falls back to the stored
// definition for a genuinely user-defined engine.
func IntrospectCommand(service string) string {
	presetName := service
	if custom, err := config.LoadCustomService(service); err == nil {
		if custom.Introspect != nil {
			return custom.Introspect.ListDatabases
		}
		if custom.Preset != "" {
			presetName = custom.Preset
		}
	}
	if p, err := config.LoadPreset(presetName); err == nil && p.Introspect != nil {
		return p.Introspect.ListDatabases
	}
	return ""
}

// ListDatabases runs the preset-declared introspection command inside the
// service container and parses its "name<TAB>size_bytes" rows. The command is
// engine-specific and lives in the service store, so this stays framework
// agnostic: it only knows how to run a command and read tab-separated output.
func ListDatabases(service, listCommand string) ([]DatabaseInfo, error) {
	if strings.TrimSpace(listCommand) == "" {
		return nil, nil
	}
	out, err := containerExec("lerd-"+service, listCommand, introspectEnv(), nil, introspectTimeout)
	if err != nil {
		return nil, fmt.Errorf("listing databases in %s: %w\n%s", service, err, strings.TrimSpace(string(out)))
	}
	return parseDatabaseRows(out), nil
}

// parseDatabaseRows turns tab-separated "name<TAB>size" lines into DatabaseInfo.
// A missing or unparseable size is treated as zero so a query that only emits
// names still lists its databases.
func parseDatabaseRows(out []byte) []DatabaseInfo {
	var dbs []DatabaseInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.SplitN(line, "\t", 2)
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		var size int64
		if len(fields) == 2 {
			size, _ = strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
		}
		dbs = append(dbs, DatabaseInfo{Name: name, SizeBytes: size})
	}
	return dbs
}

// dumpEnv carries the fixed admin password for family through the exec env so it
// never lands in argv.
func dumpEnv(family string) []string {
	if family == "postgres" {
		return []string{"PGPASSWORD=lerd"}
	}
	return []string{"MYSQL_PWD=lerd"}
}

// exportShellCommand and importShellCommand return the in-container command that
// dumps a database to stdout / loads a dump from stdin, and whether the engine
// family supports it. Mongo and other non-SQL engines are dumped with their own
// tools, out of scope here, so they report false.
func exportShellCommand(family, database string) (string, bool) {
	q := podman.ShellQuote(database)
	switch family {
	case "mysql", "mariadb":
		return "$(command -v mysqldump || command -v mariadb-dump) -uroot " + q, true
	case "postgres":
		return "pg_dump -U postgres " + q, true
	default:
		return "", false
	}
}

func importShellCommand(family, database string) (string, bool) {
	q := podman.ShellQuote(database)
	switch family {
	case "mysql", "mariadb":
		return "$(command -v mysql || command -v mariadb) --max-allowed-packet=1G -uroot " + q, true
	case "postgres":
		return "psql -U postgres -d " + q, true
	default:
		return "", false
	}
}

// ExportDatabase streams a plain SQL dump of database from the service container
// to w. The dump is uncompressed so the browser can save it directly.
func ExportDatabase(service, database string, w io.Writer) error {
	family := config.FamilyOfName(service)
	shellCmd, ok := exportShellCommand(family, database)
	if !ok {
		return fmt.Errorf("exporting %s databases is not supported", service)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dumpRestoreTimeout)
	defer cancel()
	args := []string{"exec"}
	for _, kv := range dumpEnv(family) {
		args = append(args, "--env", kv)
	}
	args = append(args, "lerd-"+service, "sh", "-c", shellCmd)
	cmd := podman.CmdContext(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("export failed: %w\n%s", err, stderr.String())
	}
	return nil
}

// ExportSnapshot streams a snapshot's stored dump to w, decompressed, so it
// downloads as a plain .sql the same as a live export rather than a .gz.
func ExportSnapshot(service, database, name string, w io.Writer) error {
	dumpPath := filepath.Join(snapshotDir(service, database, name, false), snapshotDumpFile)
	f, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("opening snapshot: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading snapshot: %w", err)
	}
	defer gz.Close()
	if _, err := io.Copy(w, gz); err != nil {
		return fmt.Errorf("streaming snapshot: %w", err)
	}
	return nil
}

// ImportDatabase streams a SQL dump from r into database on the service
// container. The database must already exist.
func ImportDatabase(service, database string, r io.Reader) error {
	family := config.FamilyOfName(service)
	shellCmd, ok := importShellCommand(family, database)
	if !ok {
		return fmt.Errorf("importing into %s databases is not supported", service)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dumpRestoreTimeout)
	defer cancel()
	args := []string{"exec", "-i"}
	for _, kv := range dumpEnv(family) {
		args = append(args, "--env", kv)
	}
	args = append(args, "lerd-"+service, "sh", "-c", shellCmd)
	cmd := podman.CmdContext(ctx, args...)
	cmd.Stdin = r
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("import failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
