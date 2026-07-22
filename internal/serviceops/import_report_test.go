package serviceops

import (
	"io"
	"os/exec"
	"testing"
)

func TestParseImportOutputClean(t *testing.T) {
	rep := parseImportOutput("SET\nCREATE TABLE\nCOPY 12\n")
	if rep.Errors != 0 || len(rep.Issues) != 0 {
		t.Fatalf("clean load reported %d errors: %+v", rep.Errors, rep.Issues)
	}
}

func TestParseImportOutputCountsPsqlErrors(t *testing.T) {
	out := `ERROR:  unrecognized configuration parameter "transaction_timeout"
ERROR:  role "root" does not exist
LINE 2:     id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
HINT:  No function matches the given name and argument types.
ERROR:  role "root" does not exist
invalid command \N
invalid command \N
invalid command \N
`
	rep := parseImportOutput(out)
	if rep.Errors != 6 {
		t.Fatalf("errors = %d, want 6", rep.Errors)
	}
	// Errors keep the order the engine hit them, since the first failure is what
	// caused the rest, and the COPY cascade trails behind as one line.
	if rep.Issues[0].Message != `ERROR:  unrecognized configuration parameter "transaction_timeout"` {
		t.Fatalf("first issue = %+v", rep.Issues[0])
	}
	if rep.Issues[1].Message != `ERROR:  role "root" does not exist` || rep.Issues[1].Count != 2 {
		t.Fatalf("second issue = %+v", rep.Issues[1])
	}
	last := rep.Issues[len(rep.Issues)-1]
	if last.Message != `invalid command \N` || last.Count != 3 {
		t.Fatalf("last issue = %+v", last)
	}
}

func TestParseImportOutputCountsMySQLErrors(t *testing.T) {
	rep := parseImportOutput("ERROR 1049 (42000): Unknown database 'nope'\n")
	if rep.Errors != 1 || rep.Issues[0].Count != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestParseImportOutputStripsPsqlLinePrefix(t *testing.T) {
	const want = `ERROR:  relation "public.audit_log" does not exist`
	rep := parseImportOutput("psql:<stdin>:412: " + want + "\npsql:<stdin>:998: " + want + "\n")
	if rep.Errors != 2 {
		t.Fatalf("errors = %d, want 2", rep.Errors)
	}
	// The same complaint from two lines of the dump counts as one issue.
	if len(rep.Issues) != 1 || rep.Issues[0].Message != want || rep.Issues[0].Count != 2 {
		t.Fatalf("issues = %+v", rep.Issues)
	}
}

func TestImportTallyCountsAcrossWrites(t *testing.T) {
	var tally ImportTally
	out := tally.Stream()
	out.Write([]byte("ERROR:  role \"root\" does not exi"))
	out.Write([]byte("st\nCREATE TABLE\ninvalid command \\N"))
	rep := tally.Report()
	if rep.Errors != 2 {
		t.Fatalf("errors = %d, want 2", rep.Errors)
	}
	if rep.Issues[0].Message != `ERROR:  role "root" does not exist` {
		t.Fatalf("issues = %+v", rep.Issues)
	}
}

// A command's stdout and stderr are copied by a goroutine each, so the tally is
// written from two goroutines at once and must hold a line from one stream
// apart from a line from the other.
func TestImportTallyAcrossConcurrentStreams(t *testing.T) {
	var tally ImportTally
	cmd := exec.Command("sh", "-c", `for i in $(seq 1 200); do echo "ERROR:  stdout boom"; echo "ERROR:  stderr boom" >&2; done`)
	cmd.Stdout = io.MultiWriter(io.Discard, tally.Stream())
	cmd.Stderr = io.MultiWriter(io.Discard, tally.Stream())
	if err := cmd.Run(); err != nil {
		t.Fatalf("running: %v", err)
	}
	rep := tally.Report()
	if rep.Errors != 400 {
		t.Fatalf("errors = %d, want 400", rep.Errors)
	}
	if len(rep.Issues) != 2 {
		t.Fatalf("issues = %+v", rep.Issues)
	}
	for _, issue := range rep.Issues {
		if issue.Count != 200 {
			t.Fatalf("issue %q counted %d, want 200", issue.Message, issue.Count)
		}
	}
}

func TestParseImportOutputCapsIssues(t *testing.T) {
	out := ""
	for i := 0; i < maxImportIssues+3; i++ {
		out += "ERROR:  failure number " + string(rune('a'+i)) + "\n"
	}
	rep := parseImportOutput(out)
	// Nothing is held back when the output carries no COPY cascade to hold it for.
	if len(rep.Issues) != maxImportIssues {
		t.Fatalf("issues = %d, want %d", len(rep.Issues), maxImportIssues)
	}
	if rep.Omitted != 3 {
		t.Fatalf("omitted = %d, want 3", rep.Omitted)
	}
	if rep.Errors != maxImportIssues+3 {
		t.Fatalf("errors = %d, want %d", rep.Errors, maxImportIssues+3)
	}
}

// The COPY cascade gets its slot only when there is one, and it never crowds
// out the failure that caused it.
func TestParseImportOutputReservesNoiseSlot(t *testing.T) {
	out := ""
	for i := 0; i < maxImportIssues+1; i++ {
		out += "ERROR:  failure number " + string(rune('a'+i)) + "\n"
	}
	out += "invalid command \\N\ninvalid command \\N\n"
	rep := parseImportOutput(out)
	if len(rep.Issues) != maxImportIssues {
		t.Fatalf("issues = %d, want %d", len(rep.Issues), maxImportIssues)
	}
	last := rep.Issues[len(rep.Issues)-1]
	if last.Message != `invalid command \N` || last.Count != 2 {
		t.Fatalf("last issue = %+v", last)
	}
	if rep.Omitted != 2 {
		t.Fatalf("omitted = %d, want 2", rep.Omitted)
	}
}

func TestImportTallyBoundsPartialLine(t *testing.T) {
	var tally ImportTally
	out := tally.Stream()
	for i := 0; i < 40; i++ {
		out.Write(make([]byte, 8<<10))
	}
	if got := len(tally.streams[0].partial); got > maxPartialLine {
		t.Fatalf("partial = %d bytes, want at most %d", got, maxPartialLine)
	}
}
