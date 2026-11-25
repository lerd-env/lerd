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

// A project in a subfolder of where the assistant was opened is named by a
// relative path as often as an absolute one.
func TestExecTest_relativeProjectRoot(t *testing.T) {
	dir := testProject(t)
	nested := filepath.Join(dir, "backend")
	for path, body := range map[string]string{
		"composer.json":          `{"require-dev":{"acme/unit":"^1.0"}}`,
		"vendor/bin/unit":        "<?php",
		"tests/InnerTest.php":    "<?php",
		"tests/Unit/OneTest.php": "<?php",
	} {
		full := filepath.Join(nested, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	whole := []string{"vendor/bin/unit"}
	inner := []string{"vendor/bin/unit", "tests/InnerTest.php"}
	for _, c := range []struct {
		args map[string]any
		root string
		argv []string
	}{
		{map[string]any{"path": "backend"}, nested, whole},
		{map[string]any{"path": "./backend/"}, nested, whole},
		{map[string]any{"path": "backend", "file": "tests/InnerTest.php"}, nested, inner},
		{map[string]any{"path": "backend/tests/InnerTest.php"}, nested, inner},
		{map[string]any{"file": "backend/tests/InnerTest.php"}, dir, []string{"vendor/bin/unit", "backend/tests/InnerTest.php"}},
		{map[string]any{"path": "backend/tests/Unit"}, nested, []string{"vendor/bin/unit", "tests/Unit"}},
		{map[string]any{"path": filepath.Join(nested, "tests/Unit")}, nested, []string{"vendor/bin/unit", "tests/Unit"}},
		{map[string]any{"path": "."}, dir, whole},
		{map[string]any{"path": "tests"}, dir, []string{"vendor/bin/unit", "tests"}},
	} {
		got := stubPHP(t, "", nil)
		if text, isErr := runTest(t, c.args); isErr || got.root != c.root || !slices.Equal(got.argv, c.argv) {
			t.Errorf("%v ran %+v, want %s %v: %s", c.args, *got, c.root, c.argv, text)
		}
	}
}

// A project without composer.json is its own root, and has no runner to find.
func TestExecTest_rootWithoutComposer(t *testing.T) {
	testProject(t)
	got := stubPHP(t, "", nil)
	if text, isErr := runTest(t, map[string]any{"path": t.TempDir()}); !isErr || !strings.Contains(text, "no test runner") || got.argv != nil {
		t.Errorf("isErr %v: %s", isErr, text)
	}
}

// The CLI runs a framework's commands with its cli_ini, and tests that need
// its memory limit must not fail only through MCP.
func TestProjectPHPArgs_carryTheFrameworksIni(t *testing.T) {
	dir := testProject(t)
	def := "name: acme\nlabel: Acme\nversion: \"1\"\npublic_dir: public\nphp:\n  cli_ini:\n    memory_limit: 512M\ndetect:\n  - file: acme.php\n"
	if err := os.WriteFile(filepath.Join(config.StoreFrameworksDir(), "acme@1.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.StoreFrameworksDir(), "index.json"),
		[]byte(`{"frameworks":[{"name":"acme","label":"Acme","versions":["1"],"latest":"1","detect":[{"file":"acme.php"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acme.php"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := projectPHPArgs("c1", dir, []string{"vendor/bin/unit"})
	want := []string{"c1", "php", "-d", "memory_limit=512M", "vendor/bin/unit"}
	if len(got) < len(want) || !slices.Equal(got[len(got)-len(want):], want) {
		t.Errorf("args = %v, want to end in %v", got, want)
	}
}

// An assistant opened in a project's subfolder is still in that project.
func TestExecTest_openedInASubfolder(t *testing.T) {
	dir := testProject(t)
	defaultSitePath = filepath.Join(dir, "tests")
	file := []string{"vendor/bin/unit", "tests/LoginTest.php"}
	for _, c := range []struct {
		args map[string]any
		argv []string
	}{
		{map[string]any{}, []string{"vendor/bin/unit"}},
		{map[string]any{"file": filepath.Join(dir, "tests/LoginTest.php")}, file},
		{map[string]any{"file": "tests/LoginTest.php"}, file},
		{map[string]any{"path": "LoginTest.php"}, file},
		{map[string]any{"path": dir}, []string{"vendor/bin/unit"}},
	} {
		got := stubPHP(t, "", nil)
		if text, isErr := runTest(t, c.args); isErr || got.root != dir || !slices.Equal(got.argv, c.argv) {
			t.Errorf("%v ran %+v, want %v: %s", c.args, *got, c.argv, text)
		}
	}
}

// A package kept inside an app is tested by the app's own runner.
func TestExecTest_nestedPackageFileRunsFromTheProject(t *testing.T) {
	dir := testProject(t)
	pkg := filepath.Join(dir, "packages", "foo")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"composer.json", "FooTest.php"} {
		if err := os.WriteFile(filepath.Join(pkg, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"vendor/bin/unit", "packages/foo/FooTest.php"}
	for _, file := range []string{"packages/foo/FooTest.php", filepath.Join(pkg, "FooTest.php")} {
		got := stubPHP(t, "", nil)
		if text, isErr := runTest(t, map[string]any{"file": file}); isErr || got.root != dir || !slices.Equal(got.argv, want) {
			t.Errorf("%s ran %+v: %s", file, *got, text)
		}
	}
}

func TestExecTest_fileOutsideTheProjectAsksForThePath(t *testing.T) {
	testProject(t)
	got := stubPHP(t, "", nil)
	text, isErr := runTest(t, map[string]any{"file": filepath.Join(t.TempDir(), "X.php")})
	if !isErr || !strings.Contains(text, "outside the project") || !strings.Contains(text, "path") || got.argv != nil {
		t.Errorf("isErr %v: %s", isErr, text)
	}
}
