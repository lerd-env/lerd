package node

import (
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The List path always passes --no-alias, so parseNvmListFull only ever sees
// installed-version lines: the current one prefixed with "->" and marked "*",
// and others indented. This mirrors real `nvm ls --no-colors --no-alias` output.
func TestParseNvmListFull_ExtractsInstalledVersions(t *testing.T) {
	raw := "->     v24.16.0 *\n" +
		"       v20.11.0\n" +
		"       v18.20.8\n"
	got := parseNvmListFull(raw)
	want := []string{"24.16.0", "20.11.0", "18.20.8"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNvmListFull() = %v, want %v", got, want)
	}
}

// nvm ls still prints a system row that can embed a resolved version; that
// must not appear as an installed nvm version in the dashboard/picker/MCP.
func TestParseNvmListFull_SkipsSystemRow(t *testing.T) {
	raw := "->     v22.14.0 *\n" +
		"         system -> v24.18.0\n" +
		"       v20.11.0\n"
	got := parseNvmListFull(raw)
	want := []string{"22.14.0", "20.11.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNvmListFull() = %v, want %v", got, want)
	}
	raw2 := "       system *\n" +
		"->     v18.20.8\n"
	got2 := parseNvmListFull(raw2)
	want2 := []string{"18.20.8"}
	if !reflect.DeepEqual(got2, want2) {
		t.Errorf("parseNvmListFull(system *) = %v, want %v", got2, want2)
	}
}

func TestParseNvmList_MajorsDeduped(t *testing.T) {
	raw := "->     v20.19.0 *\n" +
		"       v20.11.0\n" +
		"       v18.20.8\n"
	got := parseNvmList(raw)
	want := []string{"20", "18"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNvmList() = %v, want %v", got, want)
	}
}

func TestParseNvmList_Empty(t *testing.T) {
	if got := parseNvmList(""); got != nil {
		t.Errorf("parseNvmList(empty) = %v, want nil", got)
	}
	// nvm prints "N/A" when nothing is installed.
	if got := parseNvmList("            N/A\n"); got != nil {
		t.Errorf("parseNvmList(N/A) = %v, want nil", got)
	}
}

// nvmDir honours $NVM_DIR and otherwise falls back to ~/.nvm.
func TestNvmDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no persisted node.nvm_dir
	t.Setenv("NVM_DIR", "/custom/nvm")
	if got := nvmDir(); got != "/custom/nvm" {
		t.Errorf("nvmDir() with NVM_DIR = %q, want /custom/nvm", got)
	}
	t.Setenv("NVM_DIR", "")
	t.Setenv("HOME", "/home/tester")
	if got := nvmDir(); got != "/home/tester/.nvm" {
		t.Errorf("nvmDir() default = %q, want /home/tester/.nvm", got)
	}
}

func TestNvmDir_PersistedConfigWins(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NVM_DIR", "/from/env")
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.SetNodeNvmDir("/persisted/nvm")
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if got := nvmDir(); got != "/persisted/nvm" {
		t.Errorf("nvmDir() = %q, want /persisted/nvm", got)
	}
}

func TestNvmDefaultUsable(t *testing.T) {
	cases := map[string]bool{
		"":         false,
		"N/A":      false,
		"system":   false,
		"  system": false,
		"v24.18.0": true,
		"24.18.0":  true,
	}
	for in, want := range cases {
		if got := nvmDefaultUsable(in); got != want {
			t.Errorf("nvmDefaultUsable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"20":           "'20'",
		"/home/u/.nvm": "'/home/u/.nvm'",
		"a'b":          `'a'"'"'b'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
