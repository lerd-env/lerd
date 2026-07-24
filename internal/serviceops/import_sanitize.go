package serviceops

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"sync"
)

// Ownership and privilege statements name roles that only exist where the dump
// was taken. lerd's engines run one admin role, so the statements cannot apply
// and every one of them lands in the import report as an error nobody can act on.
var pgOwnership = regexp.MustCompile(`(?i)^\s*(ALTER\s+DEFAULT\s+PRIVILEGES\b|GRANT\b|REVOKE\b|ALTER\s+.*\s+OWNER\s+TO\b)`)

// The mysql families carry the same thing as a DEFINER clause on views,
// triggers and routines, which is worse: created as root it imports clean and
// only fails when the object is used. Dropping the clause defaults it to the
// importing user. Matched only on a DDL line, since row data is free text and a
// value holding the word must survive.
var (
	myDefiner = regexp.MustCompile("DEFINER=[^ ]+ ?")
	myDDL     = regexp.MustCompile(`(?i)^\s*(/\*![0-9]*\s*)?(CREATE\b|ALTER\b|DEFINER=)`)
)

// pgCreateSchema matches a plain CREATE SCHEMA so it can be made conditional.
// The form that carries schema elements is excluded, since postgres rejects
// IF NOT EXISTS on it.
var pgCreateSchema = regexp.MustCompile(`(?i)^(\s*CREATE\s+SCHEMA\s+)([^;]*;\s*)$`)

// copyStart marks the header of a postgres COPY block, after which every line is
// row data until a lone "\.", so nothing between them may be rewritten.
var copyStart = regexp.MustCompile(`(?i)^\s*COPY\s.*\sFROM\s+stdin;\s*$`)

// DumpTarget is where a dump is going, so the filter knows which statements can
// never apply and which extensions the engine could create for it.
type DumpTarget struct {
	Service    string
	Family     string
	Database   string
	Extensions []Extension
}

// ImportNotes is what the filter did on the way in, reported beside the errors
// so a dump lerd changed never reads as one that arrived untouched.
type ImportNotes struct {
	Skipped []ImportIssue
	Created []ImportIssue
}

// createExtensionFn is the container call, swapped out in tests.
var createExtensionFn = CreateExtension

// SanitizeDump filters a dump on its way to the engine so statements that name
// roles the local engine does not have never reach it, and creates a declared
// extension the moment a statement reaches for one of its types. It returns the
// filtered stream and a function reporting what it did, valid once the stream
// has been read to the end.
func SanitizeDump(t DumpTarget, r io.Reader) (io.Reader, func() ImportNotes) {
	s := &dumpSanitizer{target: t, postgres: t.Family == "postgres"}
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(s.copy(r, pw)) }()
	return pr, s.notes
}

type dumpSanitizer struct {
	target   DumpTarget
	postgres bool
	ensured  map[string]bool
	mu       sync.Mutex
	counts   map[string]int
	order    []string
	created  []ImportIssue
}

func (s *dumpSanitizer) note(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	if _, seen := s.counts[kind]; !seen {
		s.order = append(s.order, kind)
	}
	s.counts[kind]++
}

func (s *dumpSanitizer) notes() ImportNotes {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ImportNotes{Created: s.created}
	for _, kind := range s.order {
		out.Skipped = append(out.Skipped, ImportIssue{Message: kind, Count: s.counts[kind]})
	}
	return out
}

// ensureExtension creates the extension a line reaches for, before that line is
// forwarded, so the statement needing it cannot run first. Each is attempted
// once, whether or not it worked, so a dump full of one type costs one exec.
func (s *dumpSanitizer) ensureExtension(line string) {
	if !s.postgres || len(s.target.Extensions) == 0 || s.target.Database == "" {
		return
	}
	e := extensionForLine(s.target.Extensions, line)
	if e == nil || s.ensured[e.Name] {
		return
	}
	if s.ensured == nil {
		s.ensured = map[string]bool{}
	}
	s.ensured[e.Name] = true
	msg := "created extension " + e.Name + ", which the dump needs and did not carry"
	if err := createExtensionFn(s.target.Service, s.target.Database, e.Name); err != nil {
		msg = "could not create extension " + e.Name + ", which the dump needs: " + err.Error()
	}
	s.mu.Lock()
	s.created = append(s.created, ImportIssue{Message: msg, Count: 1})
	s.mu.Unlock()
}

// copy streams r to w a line at a time. ReadString rather than a Scanner: a
// mysqldump INSERT is one line and can run to megabytes, past any token limit.
func (s *dumpSanitizer) copy(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	inCopy := false
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			out := line
			switch {
			case inCopy:
				if strings.TrimRight(line, "\r\n") == `\.` {
					inCopy = false
				}
			case s.postgres && copyStart.MatchString(line):
				inCopy = true
			default:
				out = s.filter(line)
				if out != "" {
					s.ensureExtension(out)
				}
			}
			if out != "" {
				if _, werr := io.WriteString(w, out); werr != nil {
					return werr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// conditionalCreateSchema turns a bare CREATE SCHEMA into its IF NOT EXISTS
// form. Every database lerd creates already has a public schema, so a dump that
// declares one ends on an error that means nothing; the database is the same
// either way, so nothing is withheld by making it conditional.
func conditionalCreateSchema(line string) string {
	m := pgCreateSchema.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	rest := strings.ToUpper(m[2])
	if strings.HasPrefix(strings.TrimSpace(rest), "IF NOT EXISTS") || strings.Contains(rest, "CREATE ") {
		return line
	}
	return m[1] + "IF NOT EXISTS " + m[2]
}

// filter returns what should be written for one statement line, empty when the
// whole line is dropped.
func (s *dumpSanitizer) filter(line string) string {
	if s.postgres {
		if pgOwnership.MatchString(line) {
			s.note("ownership and privilege statements the local engine has no roles for")
			return ""
		}
		return conditionalCreateSchema(line)
	}
	if !myDDL.MatchString(line) || !strings.Contains(line, "DEFINER=") {
		return line
	}
	s.note("DEFINER clauses naming users the local engine does not have")
	return myDefiner.ReplaceAllString(line, "")
}
