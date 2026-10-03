package podman

import (
	"reflect"
	"testing"
)

func TestMapVMArgsTranslatesTheWorkingDirectory(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"exec -w",
			[]string{"exec", "-i", "-w", `C:\Sites\app`, "lerd-php84-fpm", "php", "artisan"},
			[]string{"exec", "-i", "-w", "/mnt/c/Sites/app", "lerd-php84-fpm", "php", "artisan"}},
		{"run --workdir",
			[]string{"run", "--workdir", "D:/x", "img"},
			[]string{"run", "--workdir", "/mnt/d/x", "img"}},
		{"posix path untouched",
			[]string{"exec", "-w", "/var/www", "c", "ls"},
			[]string{"exec", "-w", "/var/www", "c", "ls"}},
		{"nc -w is a timeout not a path",
			[]string{"exec", "lerd-nginx", "nc", "-z", "-w", "2", "10.0.0.1", "80"},
			[]string{"exec", "lerd-nginx", "nc", "-z", "-w", "2", "10.0.0.1", "80"}},
		{"other subcommands untouched",
			[]string{"logs", "-w", `C:\x`},
			[]string{"logs", "-w", `C:\x`}},
		{"trailing -w without a value",
			[]string{"exec", "-w"},
			[]string{"exec", "-w"}},
	}
	assertMapVMArgs(t, cases)
}

// A Windows path handed to the command in the container (the composer phar,
// the directory a scaffold creates) has to be the guest path too. A volume
// source is the exception: podman resolves it on the host side itself.
func TestMapVMArgsTranslatesWindowsPathArguments(t *testing.T) {
	assertMapVMArgs(t, []struct {
		name string
		in   []string
		want []string
	}{
		{"scaffold through the bundled composer",
			[]string{"exec", "-i", "-w", `C:\Users\me\Sites`, "lerd-php85-fpm", "php", `C:\Users\me\AppData\Local\lerd\bin\composer.phar`, "create-project", "laravel/laravel", `C:\Users\me\Sites\demo`},
			[]string{"exec", "-i", "-w", "/mnt/c/Users/me/Sites", "lerd-php85-fpm", "php", "/mnt/c/Users/me/AppData/Local/lerd/bin/composer.phar", "create-project", "laravel/laravel", "/mnt/c/Users/me/Sites/demo"}},
		{"volume source stays a host path",
			[]string{"run", "-v", `C:\Sites\app:/var/www`, "--volume", `D:\data:/data`, "img", `C:\Sites\app\run.php`},
			[]string{"run", "-v", `C:\Sites\app:/var/www`, "--volume", `D:\data:/data`, "img", "/mnt/c/Sites/app/run.php"}},
		{"values that only contain a path are left alone",
			[]string{"exec", "--env", `COMPOSER_HOME=C:\x`, "c", "php", "--define=a=C:/y"},
			[]string{"exec", "--env", `COMPOSER_HOME=C:\x`, "c", "php", "--define=a=C:/y"}},
	})
}

func assertMapVMArgs(t *testing.T, cases []struct {
	name string
	in   []string
	want []string
}) {
	t.Helper()
	for _, c := range cases {
		in := append([]string(nil), c.in...)
		got := mapVMArgs(in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
		if !reflect.DeepEqual(in, c.in) {
			t.Errorf("%s: input slice was modified", c.name)
		}
	}
}
