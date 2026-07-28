package node

import (
	"path/filepath"
	"strings"
	"testing"
)

// With no nvm installed anywhere, the generated fragment must still name
// $NVM_DIR/nvm.sh. The `[ -s ... ]` guard in front of it is a runtime check, so
// a unit written before nvm is set up starts working once it is; emitting an
// empty path instead bakes in a source line that can never fire.
func TestNvmSourceScriptFallsBackToCanonicalPath(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", filepath.Join(home, ".nvm"))

	prev := brewNvmScripts
	brewNvmScripts = func() []string { return nil }
	t.Cleanup(func() { brewNvmScripts = prev })

	want := filepath.Join(home, ".nvm", "nvm.sh")
	if got := sourceScript(); !strings.Contains(got, want) {
		t.Errorf("sourceScript() = %q; want it to reference %s", got, want)
	}
	if got := (nvmManager{}).ExecPrefix("20"); !strings.Contains(got, "nvm.sh") {
		t.Errorf("ExecPrefix() = %q; want it to reference nvm.sh", got)
	}
	// Detection is unaffected: nothing is installed.
	if ScriptPresent() {
		t.Error("ScriptPresent() = true with no nvm anywhere")
	}
}
