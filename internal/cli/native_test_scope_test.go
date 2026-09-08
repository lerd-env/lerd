package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nativeRuntimeTestExceptions are files that set the native runtime in a test
// and still run everywhere, because they assert through a seam that takes the
// platform rather than through the running one.
var nativeRuntimeTestExceptions = map[string]bool{
	"internal/config/php_runtime_test.go": true,
}

// TestNativeRuntimeTestsDeclareTheirPlatform fails when a test switches the
// install to the native runtime without naming macOS in its filename.
//
// The runtime is decided by the platform as well as by the stored value, so off
// macOS the answer is always container. A test that sets native and then asserts
// what follows passes here and fails on Linux CI, which has now happened three
// times. Either the file carries the platform in its name, or it asks a seam
// that takes one.
func TestNativeRuntimeTestsDeclareTheirPlatform(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		if strings.HasSuffix(path, "_darwin_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || nativeRuntimeTestExceptions[filepath.ToSlash(rel)] {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// Split so this file does not match its own search.
		assign := "Runtime = "
		if strings.Contains(string(body), assign+"config.PHPRuntime"+"Native") ||
			strings.Contains(string(body), assign+"PHPRuntime"+"Native") {
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range offenders {
		t.Errorf("%s switches the install to the native runtime but does not name macOS: rename it *_darwin_test.go, or assert through a seam that takes the platform", f)
	}
}
