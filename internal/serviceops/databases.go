package serviceops

import (
	"bufio"
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

// IntrospectCommand resolves an engine's list-databases query through the
// entity surface, so both the entities form and the legacy list_databases
// field serve it, preset first.
func IntrospectCommand(service string) string {
	spec := EntityFor(service, "databases")
	if spec == nil {
		return ""
	}
	return spec.List
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

// ExportFilename is the download name for a live export of database, from the
// declared export action's filename, with a neutral fallback for engines that
// declare none.
func ExportFilename(service, database string) string {
	return EntityExportFilename(service, "databases", database)
}

// ExportDatabase streams a dump of database from the service container to w,
// through the export action the engine's preset declares. The dump is
// uncompressed so the browser can save it directly.
func ExportDatabase(service, database string, w io.Writer) error {
	return ExportEntity(service, "databases", database, w)
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
// a truncated list never reads as the whole of what went wrong. Skipped is what
// the sanitizer held back on the way in, reported so lerd never rewrites a dump
// silently.
type ImportReport struct {
	Errors  int           `json:"errors"`
	Issues  []ImportIssue `json:"issues,omitempty"`
	Omitted int           `json:"omitted,omitempty"`
	Skipped []ImportIssue `json:"skipped,omitempty"`
	Created []ImportIssue `json:"created,omitempty"`
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

// SkippedSummary renders what the sanitizer held back, so a load that came out
// clean because lerd filtered it says so rather than looking untouched.
func (r ImportReport) SkippedSummary() string {
	if len(r.Skipped) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Skipped))
	for _, s := range r.Skipped {
		parts = append(parts, fmt.Sprintf("%d× %s", s.Count, s.Message))
	}
	return "skipped " + strings.Join(parts, "; ")
}

// CreatedSummary renders the extensions the load needed and lerd created, or
// could not, for the same reason.
func (r ImportReport) CreatedSummary() string {
	if len(r.Created) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Created))
	for _, c := range r.Created {
		parts = append(parts, c.Message)
	}
	return strings.Join(parts, "; ")
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
		if isCopyCascadeLine(msg) && (loudestNoise == "" || t.seen[msg] > t.seen[loudestNoise]) {
			loudestNoise = msg
		}
	}
	room := maxImportIssues
	if loudestNoise != "" {
		room--
	}
	rep := ImportReport{Errors: t.errors}
	for _, msg := range t.order {
		if isCopyCascadeLine(msg) {
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
	for _, marker := range []string{": ERROR", ": invalid command", ": backslash commands are restricted"} {
		if i := strings.Index(line, marker); i >= 0 {
			return line[i+2:]
		}
	}
	return line
}

func isImportErrorLine(line string) bool {
	return strings.HasPrefix(line, "ERROR") || isCopyCascadeLine(line)
}

// isCopyCascadeLine reports whether a complaint is COPY data being read as SQL,
// which is what follows a table the dump failed to create. Postgres 18 dumps
// open with \restrict, and in that mode psql refuses backslash commands, so the
// rows that used to read "invalid command \N" now say so differently.
func isCopyCascadeLine(msg string) bool {
	return strings.HasPrefix(msg, "invalid command") ||
		strings.HasPrefix(msg, "backslash commands are restricted")
}

// DumpReader unwraps a gzipped dump so a .sql.gz loads like a .sql. The engine
// clients read plain SQL: handed compressed bytes, psql reports a couple of
// encoding errors, loads nothing and still exits clean, which reads as a load
// that mostly worked. Anything else is passed through untouched, including a
// custom-format archive, which psql itself recognises and names.
func DumpReader(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(2)
	if err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return br, nil
	}
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil, fmt.Errorf("reading compressed dump: %w", err)
	}
	return gz, nil
}

// ImportOptions carries the choices a caller makes about a load. Fresh drops and
// recreates the database first, so a dump replaces what is there instead of
// colliding with it statement by statement.
type ImportOptions struct {
	Fresh bool
}

// ImportDatabase streams a dump from r into database on the service container,
// through the declared import action. The database must already exist unless
// Fresh is set. For SQL-format engines the report carries what the engine
// complained about, which is the only sign of a partial load when the client
// still exits clean, and what the sanitizer held back; other formats stream
// untouched and rely on the client's exit code.
func ImportDatabase(service, database string, r io.Reader, opt ImportOptions) (ImportReport, error) {
	spec := EntityFor(service, "databases")
	act, ok := entityAction(spec, "import")
	if !ok {
		return ImportReport{}, fmt.Errorf("importing into %s databases is not supported", service)
	}
	shellCmd, err := expandEntityCommand(act.Exec, database)
	if err != nil {
		return ImportReport{}, err
	}
	if opt.Fresh {
		if err := EmptyDatabase(service, database); err != nil {
			return ImportReport{}, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), dumpRestoreTimeout)
	defer cancel()
	args := []string{"exec", "-i"}
	for _, kv := range introspectEnv() {
		args = append(args, "--env", kv)
	}
	args = append(args, "lerd-"+service, "sh", "-c", shellCmd)
	src, err := DumpReader(r)
	if err != nil {
		return ImportReport{}, err
	}
	sqlDump := spec.Format == "sql"
	var notes func() ImportNotes
	if sqlDump {
		src, notes = SanitizeDump(DumpTarget{
			Service: service, Family: config.FamilyOfName(service), Database: database, Extensions: DeclaredExtensions(service),
		}, src)
	}
	cmd := podman.CmdContext(ctx, args...)
	cmd.Stdin = src
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ImportReport{}, fmt.Errorf("import failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if !sqlDump {
		return ImportReport{}, nil
	}
	rep := parseImportOutput(string(out))
	n := notes()
	rep.Skipped, rep.Created = n.Skipped, n.Created
	return rep, nil
}

// EmptyDatabase drops and recreates a database, the same replacement a snapshot
// restore does, so a dump lands on nothing rather than on top of what is there.
func EmptyDatabase(service, database string) error {
	if err := ValidateDatabaseName(database); err != nil {
		return err
	}
	if _, err := DropDatabase(service, database); err != nil {
		return fmt.Errorf("emptying %s: %w", database, err)
	}
	if _, err := CreateDatabase(service, database); err != nil {
		return fmt.Errorf("recreating %s: %w", database, err)
	}
	return nil
}
