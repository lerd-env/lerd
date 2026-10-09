//go:build windows

package podman

import (
	"reflect"
	"testing"
)

// PowerShell completes a script in the cwd as .\artisan, which has to reach php
// in the container as ./artisan; a namespace argument keeps its backslashes.
func TestMapVMArgsTranslatesRelativeWindowsPaths(t *testing.T) {
	assertMapVMArgs(t, []struct {
		name string
		in   []string
		want []string
	}{
		{"php .\artisan",
			[]string{"exec", "-w", `C:\Sites\app`, "lerd-php85-fpm", "php", `.\artisan`, "migrate"},
			[]string{"exec", "-w", "/mnt/c/Sites/app", "lerd-php85-fpm", "php", "./artisan", "migrate"}},
		{"namespace stays",
			[]string{"exec", "lerd-php85-fpm", "php", "artisan", "make:model", `Admin\User`},
			[]string{"exec", "lerd-php85-fpm", "php", "artisan", "make:model", `Admin\User`}},
	})
}

// The PHP container runs as root while a project under /mnt belongs to the
// machine's user, so every exec tells git to trust it (issue #2110).
func TestWithGuestEnvTrustsProjectsForGit(t *testing.T) {
	trust := []string{"--env", "GIT_CONFIG_COUNT=1", "--env", "GIT_CONFIG_KEY_0=safe.directory", "--env", "GIT_CONFIG_VALUE_0=*"}
	in := []string{"exec", "-i", "-w", "/mnt/c/Sites/app", "lerd-php85-fpm", "php", "composer.phar", "install"}
	want := append(append([]string{"exec"}, trust...), in[1:]...)
	if got := withGuestEnv(in); !reflect.DeepEqual(got, want) {
		t.Errorf("exec: got %q, want %q", got, want)
	}
	for _, args := range [][]string{{"run", "img"}, {"ps"}, nil} {
		if got := withGuestEnv(args); !reflect.DeepEqual(got, args) {
			t.Errorf("%q changed to %q", args, got)
		}
	}
}
