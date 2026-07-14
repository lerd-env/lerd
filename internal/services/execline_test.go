package services

import (
	"reflect"
	"testing"
)

func TestSplitExecStart(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "plain command",
			line: "/usr/bin/podman exec -w /Users/me/shop lerd-php84-fpm php artisan queue:work",
			want: []string{"/usr/bin/podman", "exec", "-w", "/Users/me/shop", "lerd-php84-fpm", "php", "artisan", "queue:work"},
		},
		{
			// Without quote handling podman is handed "Projects/shop'" as the
			// container name and the worker never starts (#893).
			name: "single-quoted path with spaces",
			line: "/usr/bin/podman exec -w '/Users/me/My Projects/shop' lerd-php84-fpm php artisan horizon",
			want: []string{"/usr/bin/podman", "exec", "-w", "/Users/me/My Projects/shop", "lerd-php84-fpm", "php", "artisan", "horizon"},
		},
		{
			name: "double-quoted path with spaces",
			line: `/bin/sh "/Users/me/My Data/lerd/workers/queue.sh"`,
			want: []string{"/bin/sh", "/Users/me/My Data/lerd/workers/queue.sh"},
		},
		{
			name: "escaped quote inside double quotes",
			line: `/bin/echo "a \"b\" c"`,
			want: []string{"/bin/echo", `a "b" c`},
		},
		{
			name: "runs of whitespace collapse",
			line: "  /bin/echo   a\tb  ",
			want: []string{"/bin/echo", "a", "b"},
		},
		{
			name: "empty quoted argument is kept",
			line: "/bin/echo '' x",
			want: []string{"/bin/echo", "", "x"},
		},
		{
			// ShellQuote renders an apostrophe as '\'', which only survives the
			// round trip if an escape outside quotes is honoured. Folders like
			// "Tim's Projects" are ordinary on macOS.
			name: "apostrophe in path, as ShellQuote writes it",
			line: `/usr/bin/podman exec -w '/Users/tim/Tim'\''s Projects/shop' lerd-php84-fpm php artisan queue:work`,
			want: []string{"/usr/bin/podman", "exec", "-w", "/Users/tim/Tim's Projects/shop", "lerd-php84-fpm", "php", "artisan", "queue:work"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SplitExecStart(tc.line); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitExecStart(%q)\n got %q\nwant %q", tc.line, got, tc.want)
			}
		})
	}
}
