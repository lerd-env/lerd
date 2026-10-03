//go:build windows

package podman

import "testing"

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
