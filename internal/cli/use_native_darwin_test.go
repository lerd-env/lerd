package cli

import (
	"io"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Under the native runtime there is no image to build, so `lerd use` has to
// fetch the binary instead. Without this it set a default version that nothing
// on the machine could actually serve.
func TestUseInstallsTheNativeBuild(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want bool
	}{
		{config.PHPRuntimeNative, true},
		{config.PHPRuntimeContainer, false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			cfg, err := config.LoadGlobal()
			if err != nil {
				t.Fatal(err)
			}
			cfg.PHP.Runtime = tc.mode
			if err := config.SaveGlobal(cfg); err != nil {
				t.Fatal(err)
			}

			var asked string
			orig := nativeInstallFn
			nativeInstallFn = func(version string, _ io.Writer) error {
				asked = version
				return nil
			}
			defer func() { nativeInstallFn = orig }()

			if err := runUse(nil, []string{"8.4"}); err != nil {
				t.Fatal(err)
			}
			if got := asked != ""; got != tc.want {
				t.Errorf("native install called = %v, want %v (asked %q)", got, tc.want, asked)
			}
			if tc.want && asked != "8.4" {
				t.Errorf("installed %q, want 8.4", asked)
			}
		})
	}
}
