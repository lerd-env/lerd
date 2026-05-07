package config

import (
	"strings"
	"testing"
)

func TestMySQLPresetContainsCompatDirectives(t *testing.T) {
	files := PresetFiles("mysql")
	if len(files) == 0 {
		t.Fatal("mysql preset has no file mounts")
	}

	cnf := files[0].Content

	for _, directive := range []string{
		"mysql-native-password=ON",
		"authentication_policy='mysql_native_password,,'",
		"restrict-fk-on-non-standard-key=OFF",
	} {
		if !strings.Contains(cnf, directive) {
			t.Errorf("mysql lerd.cnf missing %q", directive)
		}
	}
}
