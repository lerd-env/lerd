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
