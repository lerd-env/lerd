package mcp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// maxTestOutput keeps a failing suite from filling a small model's context.
// Runners print their summary last, so most of it is kept from the end.
const (
	maxTestOutput     = 12000
	testOutputHead    = 3000
	testOutputOmitted = "\n\n[... output cut, run one file or filter to see it all ...]\n\n"
)

// runProjectPHPFn is a seam so tests need no container.
var runProjectPHPFn = runProjectPHP

func testTool() mcpTool {
	return mcpTool{
		Name:        "test",
		Description: "Run the project's tests (Pest, PHPUnit…). No arguments runs all.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"file":   {Type: "string", Description: "Test file or folder, e.g. tests/Feature/LoginTest.php."},
				"filter": {Type: "string", Description: "Test name to match."},
				"path":   {Type: "string", Description: "Project root. Defaults to cwd."},
			},
		},
	}
}

func execTest(args map[string]any) (any, *rpcError) {
	root, file, err := testTarget(args)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	runner, err := config.TestRunnerForDir(root)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if runner == nil {
		return toolErr(fmt.Sprintf("lerd knows no test runner for %s: install one with exec composer (require --dev pestphp/pest or phpunit/phpunit), or run the project's own with exec vendor_run", root)), nil
	}
	argv, err := runner.Argv(file, strArg(args, "filter"))
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if script := argv[1]; strings.Contains(script, "/") {
		if _, err := os.Stat(filepath.Join(root, script)); err != nil {
			return toolErr(fmt.Sprintf("%s is missing: install the project's dependencies with exec composer install", script)), nil
		}
	}

	start := time.Now()
	out, errBody, runErr := runProjectPHPFn(root, argv[1:])
	if errBody != nil {
		return errBody, nil
	}
	line := fmt.Sprintf("in %s: %s\n", time.Since(start).Round(100*time.Millisecond), strings.Join(argv, " "))
	body := line + cutTestOutput(out)
	if runErr == nil {
		return toolOK("OK (exit 0) " + body), nil
	}
	// Failing tests are an answer, not a broken tool. podman's own failures,
	// and a process killed by a signal, start at 125.
	var ee *exec.ExitError
	if errors.As(runErr, &ee) && ee.ExitCode() > 0 && ee.ExitCode() < 125 {
		return toolOK(fmt.Sprintf("FAILED (exit %d) %s", ee.ExitCode(), body)), nil
	}
	return toolErr(fmt.Sprintf("the run did not finish: %v\n%s", runErr, body)), nil
}

// testTarget reads the project and the test file in it. The project is the
// closest one at or above path, and a path below it names the file: models hand
// the file over as path often enough.
func testTarget(args map[string]any) (root, file string, err error) {
	path, file := strArg(args, "path"), strArg(args, "file")
	base := defaultSitePath
	if filepath.IsAbs(path) {
		base = path
	} else if base != "" {
		base = filepath.Join(base, path)
	}
	if base == "" {
		return "", "", errors.New("path is required: pass the project root, or open your assistant in it")
	}
	root = projectRootOf(base)
	if file == "" && path != "" && filepath.Clean(base) != root {
		file = base
	}
	if file == "" {
		return root, "", nil
	}
	if strings.HasPrefix(file, "-") {
		return "", "", fmt.Errorf("file %q looks like an option; pass a test file or folder, and a test name as filter", file)
	}
	if filepath.IsAbs(file) {
		if rel, relErr := filepath.Rel(root, file); relErr == nil {
			file = rel
		}
	}
	if !filepath.IsLocal(file) {
		return "", "", fmt.Errorf("file %q is outside the project %s; pass the project it is in as path", file, root)
	}
	if _, statErr := os.Stat(filepath.Join(root, file)); statErr != nil {
		return "", "", fmt.Errorf("no file %s in %s; pass a test name as filter instead", file, root)
	}
	return root, file, nil
}

// projectRootOf is path itself or the closest folder above it that is a project,
// and path when none is. A linked site counts without a composer.json.
func projectRootOf(path string) string {
	path = filepath.Clean(path)
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
			return dir
		}
		if _, err := config.FindSiteByPath(dir); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			return path
		}
	}
}

func cutTestOutput(out string) string {
	if len(out) <= maxTestOutput {
		return out
	}
	tail := out[len(out)-(maxTestOutput-testOutputHead):]
	return strings.ToValidUTF8(out[:testOutputHead]+testOutputOmitted+tail, "")
}
