package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFrameworkCLIIniParsesFromYAML(t *testing.T) {
	var fw Framework
	src := "name: magento\nphp:\n  min: \"8.1\"\n  cli_ini:\n    memory_limit: \"-1\"\n"
	if err := yaml.Unmarshal([]byte(src), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.PHP.CLIIni["memory_limit"] != "-1" {
		t.Fatalf("got %v", fw.PHP.CLIIni)
	}
	if fw.PHP.Min != "8.1" {
		t.Fatalf("min lost: %q", fw.PHP.Min)
	}
}

func TestValidatePHPIni(t *testing.T) {
	cases := []struct {
		name    string
		ini     map[string]string
		wantErr bool
	}{
		{"empty", nil, false},
		{"unlimited memory", map[string]string{"memory_limit": "-1"}, false},
		{"sized memory", map[string]string{"memory_limit": "756M"}, false},
		{"dotted directive", map[string]string{"opcache.enable": "1"}, false},
		{"path value", map[string]string{"error_log": "/var/log/php.log"}, false},

		// Each directive becomes one `-d name=value` argv entry.
		{"equals splits the pair", map[string]string{"memory_limit": "-1=x"}, true},
		{"space splits the arg", map[string]string{"memory_limit": "-1 -d auto_prepend_file=/tmp/x"}, true},
		{"tab", map[string]string{"memory_limit": "-1\tx"}, true},
		{"newline", map[string]string{"memory_limit": "-1\nfoo=1"}, true},
		{"nul", map[string]string{"memory_limit": "-1\x00"}, true},

		{"bad directive name", map[string]string{"memory limit": "-1"}, true},
		{"directive with equals", map[string]string{"memory=limit": "-1"}, true},
		{"empty directive", map[string]string{"": "1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePHPIni(tc.ini)
			if tc.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil, got %v", err)
			}
		})
	}
}

// auto_prepend_file makes every PHP process execute a file from the repo, so an
// embedded framework_def from a cloned repo must never reach it.
func TestSanitizeProjectFrameworkDefStripsCLIIni(t *testing.T) {
	def := &Framework{
		Name: "evil",
		PHP:  FrameworkPHP{Min: "8.1", CLIIni: map[string]string{"auto_prepend_file": "/tmp/pwn.php"}},
	}
	safe := SanitizeProjectFrameworkDef(def)
	if safe.PHP.CLIIni != nil {
		t.Fatalf("cli ini survived sanitize: %v", safe.PHP.CLIIni)
	}
	if safe.PHP.Min != "8.1" {
		t.Error("the inert php range should survive")
	}
	if def.PHP.CLIIni == nil {
		t.Error("sanitize mutated the caller's definition")
	}
}
