package cli

import (
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestRunTests(t *testing.T) {
	var php, console []string
	prevPHP, prevConsole := testRunPHPFn, testRunConsoleFn
	testRunPHPFn = func(_ string, a []string) error { php = a; return nil }
	testRunConsoleFn = func(a []string) error { console = a; return nil }
	t.Cleanup(func() { testRunPHPFn, testRunConsoleFn = prevPHP, prevConsole })

	r := &config.TestRunner{Command: "php vendor/bin/unit run", Filter: "--filter={filter}"}
	if err := runTests("/p", r, []string{"--filter", "x", "tests/A.php"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(php, []string{"vendor/bin/unit", "run", "--filter", "x", "tests/A.php"}) || console != nil {
		t.Errorf("php %v console %v", php, console)
	}
	php = nil
	if err := runTests("/p", nil, []string{"--parallel"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(console, []string{"test", "--parallel"}) || php != nil {
		t.Errorf("no runner: php %v console %v", php, console)
	}
}
