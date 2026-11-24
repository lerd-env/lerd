package mcp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// testProject is a plain composer project whose package declares a runner.
func testProject(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(config.StoreFrameworksDir(), "index.json"), `{"frameworks":[],"packages":[{"name":"acme/unit"}]}`)
	write(config.StorePackageFile("acme/unit", ""), "package: acme/unit\ntest:\n  command: php vendor/bin/unit\n  filter: --filter={filter}\n")
	dir := t.TempDir()
	write(filepath.Join(dir, "composer.json"), `{"require-dev":{"acme/unit":"^1.0"}}`)
	write(filepath.Join(dir, "vendor/bin/unit"), "<?php")
	write(filepath.Join(dir, "tests/LoginTest.php"), "<?php")
	prev := defaultSitePath
	defaultSitePath = dir
	t.Cleanup(func() { defaultSitePath = prev })
	return dir
}

type phpRun struct {
	root string
	argv []string
}

func stubPHP(t *testing.T, out string, err error) *phpRun {
	t.Helper()
	got := &phpRun{}
	prev := runProjectPHPFn
	runProjectPHPFn = func(root string, argv []string) (string, any, error) {
		*got = phpRun{root, argv}
		return out, nil, err
	}
	t.Cleanup(func() { runProjectPHPFn = prev })
	return got
}

func runTest(t *testing.T, args map[string]any) (text string, isErr bool) {
	t.Helper()
	res, rpcErr := execTest(args)
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	m := res.(map[string]any)
	isErr, _ = m["isError"].(bool)
	return m["content"].([]map[string]any)[0]["text"].(string), isErr
}

func exitErr(t *testing.T, code string) error {
	t.Helper()
	return exec.Command("sh", "-c", "exit "+code).Run()
}

func TestExecTest_runsTheWholeSuite(t *testing.T) {
	dir := testProject(t)
	got := stubPHP(t, "OK (3 tests)", nil)
	text, isErr := runTest(t, map[string]any{})
	if isErr || !strings.HasPrefix(text, "OK (exit 0)") || !strings.Contains(text, "OK (3 tests)") {
		t.Errorf("isErr %v: %s", isErr, text)
	}
	if got.root != dir || !slices.Equal(got.argv, []string{"vendor/bin/unit"}) {
		t.Errorf("ran %+v", *got)
	}
}

func TestExecTest_fileAndFilter(t *testing.T) {
	dir := testProject(t)
	want := []string{"vendor/bin/unit", "--filter=it logs in", "tests/LoginTest.php"}
	for _, args := range []map[string]any{
		{"file": "tests/LoginTest.php", "filter": "it logs in"},
		{"path": dir, "file": "tests/LoginTest.php", "filter": "it logs in"},
		{"path": dir, "file": filepath.Join(dir, "tests/LoginTest.php"), "filter": "it logs in"},
		// The file handed over as path, in both shapes.
		{"path": "tests/LoginTest.php", "filter": "it logs in"},
		{"path": filepath.Join(dir, "tests/LoginTest.php"), "filter": "it logs in"},
	} {
		got := stubPHP(t, "", nil)
		if text, isErr := runTest(t, args); isErr || !slices.Equal(got.argv, want) || got.root != dir {
			t.Errorf("%v ran %+v: %s", args, *got, text)
		}
	}
}

func TestExecTest_failingTestsAreAnAnswer(t *testing.T) {
	testProject(t)
	stubPHP(t, "1 failed", exitErr(t, "2"))
	if text, isErr := runTest(t, map[string]any{}); isErr || !strings.HasPrefix(text, "FAILED (exit 2)") || !strings.Contains(text, "1 failed") {
		t.Errorf("isErr %v: %s", isErr, text)
	}
}

func TestExecTest_aRunThatDidNotFinishIsAnError(t *testing.T) {
	for _, err := range []error{errors.New("cancelled by the client"), exitErr(t, "137"), exitErr(t, "125")} {
		testProject(t)
		stubPHP(t, "partial", err)
		if text, isErr := runTest(t, map[string]any{}); !isErr || !strings.Contains(text, "partial") {
			t.Errorf("%v: isErr %v: %s", err, isErr, text)
		}
	}
}

func TestExecTest_refuses(t *testing.T) {
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"missing file": {map[string]any{"file": "tests/Nope.php"}, "no file tests/Nope.php"},
		"outside":      {map[string]any{"file": "../x.php"}, "outside the project"},
		"option":       {map[string]any{"file": "--version"}, "looks like an option"},
	} {
		testProject(t)
		got := stubPHP(t, "", nil)
		text, isErr := runTest(t, c.args)
		if !isErr || !strings.Contains(text, c.want) || got.argv != nil {
			t.Errorf("%s: isErr %v ran %v: %s", name, isErr, got.argv, text)
		}
	}
}

func TestExecTest_dependenciesNotInstalled(t *testing.T) {
	dir := testProject(t)
	if err := os.Remove(filepath.Join(dir, "vendor/bin/unit")); err != nil {
		t.Fatal(err)
	}
	if text, isErr := runTest(t, map[string]any{}); !isErr || !strings.Contains(text, "composer install") {
		t.Errorf("isErr %v: %s", isErr, text)
	}
}

func TestExecTest_noRunner(t *testing.T) {
	dir := testProject(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := runTest(t, map[string]any{}); !isErr || !strings.Contains(text, "no test runner") {
		t.Errorf("isErr %v: %s", isErr, text)
	}
}

func TestExecTest_longOutputKeepsTheSummary(t *testing.T) {
	testProject(t)
	stubPHP(t, "FIRST"+strings.Repeat("x", 50000)+"SUMMARY", exitErr(t, "1"))
	text, _ := runTest(t, map[string]any{})
	if len(text) > maxTestOutput+500 || !strings.Contains(text, "FIRST") || !strings.HasSuffix(text, "SUMMARY") || !strings.Contains(text, "output cut") {
		t.Errorf("%d bytes, %.60s", len(text), text)
	}
}

func TestTestTool_isCallableByName(t *testing.T) {
	testProject(t)
	stubPHP(t, "fine", nil)
	res, rpcErr := handleToolCall([]byte(`{"name":"test","arguments":{}}`))
	if rpcErr != nil || !strings.Contains(res.(map[string]any)["content"].([]map[string]any)[0]["text"].(string), "OK (exit 0)") {
		t.Errorf("res = %v, err = %v", res, rpcErr)
	}
}
