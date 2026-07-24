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
	"sync"
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
		return mysqlDumpBin + " -uroot " + strings.Join(DumpFlags(family), " ") + " " + q, true
	case "postgres":
		return "pg_dump -U postgres " + strings.Join(DumpFlags(family), " ") + " " + q, true
	default:
		return "", false
	}
}

// DumpFlags are the flags every dump of a family carries, so an export, a
// snapshot and a migration all produce the same file. Postgres needs --clean
// --if-exists to load back over objects that already exist, which mysqldump does
// on its own; mysqldump needs --routines and --events, which are off by default
// and would otherwise leave stored procedures out of the dump without a word.
func DumpFlags(family string) []string {
	if family == "postgres" {
		return []string{"--clean", "--if-exists"}
	}
	return []string{"--single-transaction", "--quick", "--no-tablespaces", "--routines", "--triggers", "--events"}
}

func importShellCommand(family, database string) (string, bool) {
	q := podman.ShellQuote(database)
	switch family {
	case "mysql", "mariadb":
		return mysqlClientBin + " --max-allowed-packet=1G -uroot " + q, true
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
	if err := ValidateDatabaseName(database); err != nil {
		return err
	}
	clean, err := sanitizeSnapshotName(name)
	if err != nil {
		return err
	}
	dumpPath := filepath.Join(snapshotDir(service, database, clean, false), snapshotDumpFile)
	f, openErr := os.Open(dumpPath)
	if openErr != nil {
		return fmt.Errorf("opening snapshot: %w", openErr)
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

// maxImportIssues caps the distinct complaints kept from a load. A dump replayed
// over a populated schema complains once per object, so the cap is high enough to
// carry the whole list to the UI and still bounded well under a 27k-line psql
// transcript.
const maxImportIssues = 500

// summaryIssues caps what the one-line CLI summary spells out. The rest is
// counted in the same "and N more" tail, since the full list belongs in the UI.
const summaryIssues = 5

// ImportIssue is one distinct complaint the engine made, and how often it made
// it. ImportReport summarises a load: psql exits 0 even when every statement in
// a dump fails, so without counting its output a dump can half land and still
// look like a clean import.
type ImportIssue struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// Omitted is how many further distinct complaints were dropped past the cap, so
// a truncated list never reads as the whole of what went wrong.
type ImportReport struct {
	Errors  int           `json:"errors"`
	Issues  []ImportIssue `json:"issues,omitempty"`
	Omitted int           `json:"omitted,omitempty"`
}

// Summary renders the report as one line for the CLI and the phase stream.
func (r ImportReport) Summary() string {
	if r.Errors == 0 {
		return ""
	}
	shown, more := r.Issues, r.Omitted
	if len(shown) > summaryIssues {
		more += len(shown) - summaryIssues
		shown = shown[:summaryIssues]
	}
	parts := make([]string, 0, len(shown)+1)
	for _, issue := range shown {
		parts = append(parts, fmt.Sprintf("%d× %s", issue.Count, issue.Message))
	}
	if more > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", more))
	}
	return fmt.Sprintf("the engine reported %d errors: %s", r.Errors, strings.Join(parts, "; "))
}

// ImportTally counts an engine's complaints as its output streams past, so a
// caller can hand the output straight to a terminal and still summarise it at
// the end. It counts psql and mysql error lines, plus psql's "invalid command"
// lines, which are what a COPY block turns into once its table failed to exist.
type ImportTally struct {
	mu      sync.Mutex
	streams []*tallyStream
	seen    map[string]int
	order   []string
	errors  int
}

// maxPartialLine bounds the unterminated tail a stream carries between writes,
// so output that never sends a newline cannot grow it without limit.
const maxPartialLine = 64 << 10

// Stream returns a writer for one of a command's output streams. Each stream
// keeps its own unterminated tail, because os/exec copies stdout and stderr on
// a goroutine each and one shared tail would splice fragments of one line onto
// the other.
func (t *ImportTally) Stream() io.Writer {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := &tallyStream{tally: t}
	t.streams = append(t.streams, s)
	return s
}

type tallyStream struct {
	tally   *ImportTally
	partial string
}

func (s *tallyStream) Write(p []byte) (int, error) {
	s.tally.mu.Lock()
	defer s.tally.mu.Unlock()
	text := s.partial + string(p)
	lines := strings.Split(text, "\n")
	s.partial = lines[len(lines)-1]
	if len(s.partial) > maxPartialLine {
		s.partial = s.partial[:maxPartialLine]
	}
	for _, line := range lines[:len(lines)-1] {
		s.tally.add(line)
	}
	return len(p), nil
}

// add records one complete line. The caller holds the tally's lock.
func (t *ImportTally) add(line string) {
	line = trimImportPrefix(strings.TrimRight(line, "\r "))
	if !isImportErrorLine(line) {
		return
	}
	if t.seen == nil {
		t.seen = map[string]int{}
	}
	if _, ok := t.seen[line]; !ok {
		t.order = append(t.order, line)
	}
	t.seen[line]++
	t.errors++
}

// Report closes off whatever line was still in flight and returns the summary.
// Errors are kept in the order the engine hit them, because the first failures
// are what caused the later ones, and the cascade of unparsed COPY data that
// follows a missing table is folded into a single loudest line so it cannot
// crowd the cause out of the list.
func (t *ImportTally) Report() ImportReport {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.streams {
		if s.partial != "" {
			t.add(s.partial)
			s.partial = ""
		}
	}
	loudestNoise := ""
	for _, msg := range t.order {
		if strings.HasPrefix(msg, "invalid command") && (loudestNoise == "" || t.seen[msg] > t.seen[loudestNoise]) {
			loudestNoise = msg
		}
	}
	room := maxImportIssues
	if loudestNoise != "" {
		room--
	}
	rep := ImportReport{Errors: t.errors}
	for _, msg := range t.order {
		if strings.HasPrefix(msg, "invalid command") {
			continue
		}
		if len(rep.Issues) == room {
			rep.Omitted++
			continue
		}
		rep.Issues = append(rep.Issues, ImportIssue{Message: msg, Count: t.seen[msg]})
	}
	if loudestNoise != "" {
		rep.Issues = append(rep.Issues, ImportIssue{Message: loudestNoise, Count: t.seen[loudestNoise]})
	}
	return rep
}

func parseImportOutput(out string) ImportReport {
	var tally ImportTally
	_, _ = tally.Stream().Write([]byte(out))
	return tally.Report()
}

// trimImportPrefix drops psql's "psql:<stdin>:412: " location prefix so the same
// complaint from different lines of a dump counts as one issue.
func trimImportPrefix(line string) string {
	if !strings.HasPrefix(line, "psql:") {
		return line
	}
	for _, marker := range []string{": ERROR", ": invalid command"} {
		if i := strings.Index(line, marker); i >= 0 {
			return line[i+2:]
		}
	}
	return line
}

func isImportErrorLine(line string) bool {
	return strings.HasPrefix(line, "ERROR") || strings.HasPrefix(line, "invalid command")
}

// ImportDatabase streams a SQL dump from r into database on the service
// container. The database must already exist. The report carries what the
// engine complained about, which is the only sign of a partial load when the
// client still exits clean.
func ImportDatabase(service, database string, r io.Reader) (ImportReport, error) {
	family := config.FamilyOfName(service)
	shellCmd, ok := importShellCommand(family, database)
	if !ok {
		return ImportReport{}, fmt.Errorf("importing into %s databases is not supported", service)
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ImportReport{}, fmt.Errorf("import failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return parseImportOutput(string(out)), nil
}
