package serviceops

import "testing"

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

func TestParseImportOutputKeepsPsqlLinePrefix(t *testing.T) {
	rep := parseImportOutput("psql:<stdin>:412: ERROR:  relation \"public.audit_log\" does not exist\n")
	if rep.Errors != 1 {
		t.Fatalf("errors = %d, want 1", rep.Errors)
	}
}

func TestImportTallyCountsAcrossWrites(t *testing.T) {
	var tally ImportTally
	tally.Write([]byte("ERROR:  role \"root\" does not exi"))
	tally.Write([]byte("st\nCREATE TABLE\ninvalid command \\N"))
	rep := tally.Report()
	if rep.Errors != 2 {
		t.Fatalf("errors = %d, want 2", rep.Errors)
	}
	if rep.Issues[0].Message != `ERROR:  role "root" does not exist` {
		t.Fatalf("issues = %+v", rep.Issues)
	}
}

func TestParseImportOutputCapsIssues(t *testing.T) {
	out := ""
	for i := 0; i < maxImportIssues+3; i++ {
		out += "ERROR:  failure number " + string(rune('a'+i)) + "\n"
	}
	rep := parseImportOutput(out)
	// One slot is held back for the COPY cascade, which this output has none of.
	if len(rep.Issues) != maxImportIssues-1 {
		t.Fatalf("issues = %d, want %d", len(rep.Issues), maxImportIssues-1)
	}
	if rep.Errors != maxImportIssues+3 {
		t.Fatalf("errors = %d, want %d", rep.Errors, maxImportIssues+3)
	}
}
