package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `brew install nvm` puts nvm.sh under the Homebrew prefix and tells the user
// to create an empty ~/.nvm for the versions. NVM_DIR therefore holds no
// nvm.sh, and looking only there reports a machine with nvm as having none.
func brewNvmLayout(t *testing.T) (home, script string) {
	t.Helper()
	tmp := t.TempDir()
	home = filepath.Join(tmp, "home")
	brewOpt := filepath.Join(tmp, "homebrew", "opt", "nvm")
	if err := os.MkdirAll(filepath.Join(home, ".nvm", "versions", "node"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(brewOpt, 0755); err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(brewOpt, "nvm.sh")
	if err := os.WriteFile(script, []byte("# nvm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", filepath.Join(home, ".nvm"))

	prev := brewNvmScripts
	brewNvmScripts = func() []string { return []string{script} }
	t.Cleanup(func() { brewNvmScripts = prev })
	return home, script
}

func TestNvmAvailableWithHomebrewLayout(t *testing.T) {
	brewNvmLayout(t)

	if !ScriptPresent() {
		t.Error("ScriptPresent() = false; want a brew-installed nvm detected")
	}
	if !(nvmManager{}).Available() {
		t.Error("nvmManager.Available() = false; want the brew-installed nvm usable")
	}
}

// The sourced script must be the brew one while NVM_DIR stays the versions dir,
// which is exactly what Homebrew's own caveats tell users to configure.
func TestNvmSourceScriptUsesBrewScript(t *testing.T) {
	home, script := brewNvmLayout(t)

	got := sourceScript()
	if !strings.Contains(got, script) {
		t.Errorf("sourceScript() = %q; want it to source %s", got, script)
	}
	if !strings.Contains(got, filepath.Join(home, ".nvm")) {
		t.Errorf("sourceScript() = %q; want NVM_DIR left at the versions dir", got)
	}
}

// A plain script install keeps sourcing $NVM_DIR/nvm.sh.
func TestNvmSourceScriptPrefersNvmDirScript(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	nvm := filepath.Join(home, ".nvm")
	if err := os.MkdirAll(nvm, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvm, "nvm.sh"), []byte("# nvm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", nvm)

	if got := sourceScript(); !strings.Contains(got, filepath.Join(nvm, "nvm.sh")) {
		t.Errorf("sourceScript() = %q; want the NVM_DIR script", got)
	}
}

func TestScriptPresentFalseWithoutNvm(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("NVM_DIR", filepath.Join(tmp, "home", ".nvm"))

	prev := brewNvmScripts
	brewNvmScripts = func() []string { return nil }
	t.Cleanup(func() { brewNvmScripts = prev })

	if ScriptPresent() {
		t.Error("ScriptPresent() = true with no nvm anywhere")
	}
}
