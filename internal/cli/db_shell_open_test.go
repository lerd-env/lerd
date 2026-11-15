package cli

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// fakeShell stands in for the database client: each run answers with the next
// script, writing its output where the real client would.
type fakeShell struct {
	runs    int
	scripts []func(stdout, stderr io.Writer) error
}

func (f *fakeShell) run(cmd *exec.Cmd) error {
	s := f.scripts[f.runs]
	f.runs++
	return s(cmd.Stdout, cmd.Stderr)
}

func stubDbShell(t *testing.T, interactive, confirm bool, scripts ...func(io.Writer, io.Writer) error) (*fakeShell, *[]string) {
	t.Helper()
	sh := &fakeShell{scripts: scripts}
	var created []string
	prevRun, prevTTY, prevConfirm, prevCreate := dbShellRun, dbShellInteractive, dbShellConfirm, createDatabase
	dbShellRun = sh.run
	dbShellInteractive = func() bool { return interactive }
	dbShellConfirm = func(string, bool) bool { return confirm }
	createDatabase = func(svc, name string) (bool, error) { created = append(created, svc+"/"+name); return true, nil }
	t.Cleanup(func() {
		dbShellRun, dbShellInteractive, dbShellConfirm, createDatabase = prevRun, prevTTY, prevConfirm, prevCreate
	})
	return sh, &created
}

var exitOne = errors.New("exit status 1")

func says(stream int, text string) func(io.Writer, io.Writer) error {
	return func(stdout, stderr io.Writer) error {
		w := stderr
		if stream == 1 {
			w = stdout
		}
		io.WriteString(w, text)
		return exitOne
	}
}

func fine(io.Writer, io.Writer) error { return nil }

var shopOnMySQL = &dbEnv{service: "mysql", connection: "mysql", database: "shop", username: "root", password: "lerd"}

// The shell opens straight away: no query asks first whether the database is there.
func TestOpenDbShell_runsWithoutAskingFirst(t *testing.T) {
	sh, created := stubDbShell(t, false, false, fine)
	if err := openDbShell(shopOnMySQL); err != nil {
		t.Fatal(err)
	}
	if sh.runs != 1 || len(*created) != 0 {
		t.Errorf("runs=%d created=%v, want one run and nothing created", sh.runs, *created)
	}
}

// Off a terminal, a missing database is reported with the command that makes it.
func TestOpenDbShell_namesTheFixWhenTheDatabaseIsMissing(t *testing.T) {
	for _, tc := range []struct {
		env  *dbEnv
		text string
	}{
		{shopOnMySQL, "ERROR 1049 (42000): Unknown database 'shop'\n"},
		{&dbEnv{service: "postgres", connection: "pgsql", database: "shop", username: "postgres"}, "psql: error: connection to server failed: FATAL:  database \"shop\" does not exist\n"},
	} {
		sh, created := stubDbShell(t, false, true, says(2, tc.text))
		err := openDbShell(tc.env)
		if err == nil || !strings.Contains(err.Error(), "lerd db:create shop") {
			t.Errorf("%s: err = %v, want it to name lerd db:create shop", tc.env.service, err)
		}
		if sh.runs != 1 || len(*created) != 0 {
			t.Errorf("%s: runs=%d created=%v, want one run and nothing created", tc.env.service, sh.runs, *created)
		}
	}
}

// On a terminal, where the client's error comes back on the pty, the user is
// offered the database and the shell opens again once it exists.
func TestOpenDbShell_offersToCreateItOnATerminal(t *testing.T) {
	sh, created := stubDbShell(t, true, true, says(1, "ERROR 1049 (42000): Unknown database 'shop'\r\n"), fine)
	if err := openDbShell(shopOnMySQL); err != nil {
		t.Fatal(err)
	}
	if sh.runs != 2 || len(*created) != 1 || (*created)[0] != "mysql/shop" {
		t.Errorf("runs=%d created=%v, want the database made and the shell opened again", sh.runs, *created)
	}

	sh, created = stubDbShell(t, true, false, says(1, "ERROR 1049 (42000): Unknown database 'shop'\r\n"))
	if err := openDbShell(shopOnMySQL); err == nil || sh.runs != 1 || len(*created) != 0 {
		t.Errorf("declined: err=%v runs=%d created=%v, want an error and nothing made", err, sh.runs, *created)
	}
}

// Any other failure, or another database's name in the message, is the
// client's own and passes through untouched.
func TestOpenDbShell_leavesOtherFailuresAlone(t *testing.T) {
	for _, text := range []string{
		"ERROR 1045 (28000): Access denied for user 'root'\n",
		"ERROR 1049 (42000): Unknown database 'other'\n",
		"ERROR 1146 (42S02): Table 'shop.users' doesn't exist\n",
	} {
		sh, created := stubDbShell(t, true, true, says(2, text))
		err := openDbShell(shopOnMySQL)
		if !errors.Is(err, exitOne) || sh.runs != 1 || len(*created) != 0 {
			t.Errorf("%q: err=%v runs=%d created=%v, want the client's own failure untouched", text, err, sh.runs, *created)
		}
	}
}
