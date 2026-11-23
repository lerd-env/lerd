package ui

import (
	"strings"
	"testing"
)

func TestFailureMessage(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{
			// What `lerd env` actually prints for a project with no framework:
			// the reason on the ✗ line, then the guidance under it. Taking only
			// the last line dropped the reason and kept the guidance.
			name: "a failure marker carries the reason and its guidance",
			out: " ✗ no framework detected for this site\n" +
				"Define one with 'lerd framework add' or add a framework YAML to /home/me/.config/lerd/frameworks\n",
			want: "no framework detected for this site — Define one with 'lerd framework add' " +
				"or add a framework YAML to /home/me/.config/lerd/frameworks",
		},
		{
			name: "progress output above the failure is left out",
			out:  " → configuring .env…\n → detecting services…\n ✗ reading .env: permission denied\n",
			want: "reading .env: permission denied",
		},
		{
			name: "without a marker the last line is the best available",
			out:  "some output\nfinal problem\n",
			want: "final problem",
		},
		{
			name: "a single line with no marker is used as is",
			out:  "exit status 1",
			want: "exit status 1",
		},
		{
			// What a setup step prints when the PHP it runs on is missing: the
			// reason from the child, then setup's own wrapper around its exit.
			name: "a bare exit status leads with the reason the step printed above it",
			out: "→ composer install\n\n" +
				" ✗ PHP 8.6 is not installed (/home/me/.local/share/lerd/bin/php-native-8.6)\n\n\n" +
				" ✗ composer install: exit status 1\n",
			want: "PHP 8.6 is not installed (/home/me/.local/share/lerd/bin/php-native-8.6) — composer install: exit status 1",
		},
		{
			name: "a bare exit status after unmarked output leads with its last line",
			out: "→ composer install\n" +
				"Your requirements could not be resolved to an installable set of packages.\n" +
				"  Problem 1\n" +
				"    - Root composer.json requires php ^8.6 but your php version (8.5.1) does not satisfy that requirement.\n" +
				" ✗ composer install: exit status 2\n",
			want: "- Root composer.json requires php ^8.6 but your php version (8.5.1) does not satisfy that requirement. — composer install: exit status 2",
		},
		{
			// What Composer prints for an invalid composer.json, usage line
			// trimmed. The usage line comes last, so the reason is under the heading.
			name: "a bare exit status after a Composer error leads with its message",
			out: "→ composer install\n\n" +
				"In JsonFile.php line 401:\n" +
				"                                                                    \n" +
				"  \"./composer.json\" does not contain valid JSON                     \n" +
				"  Parse error on line 4:                                            \n" +
				"  Expected: 'STRING' - It appears you have an extra trailing comma  \n" +
				"                                                                    \n\n" +
				"install [--prefer-source] [--prefer-dist] [--dry-run] [--no-dev] [--] [<packages>...]\n\n" +
				" ✗ composer install: exit status 1\n",
			want: "\"./composer.json\" does not contain valid JSON Parse error on line 4: " +
				"Expected: 'STRING' - It appears you have an extra trailing comma — composer install: exit status 1",
		},
		{
			name: "a long reason is cut short so the failing step is still named",
			out: "→ composer install\n" +
				"In JsonFile.php line 401:\n" +
				"  \"./composer.json\" does not contain valid JSON " + strings.Repeat("x", 400) + "\n" +
				" ✗ composer install: exit status 1\n",
			want: "\"./composer.json\" does not contain valid JSON " + strings.Repeat("x", 153) + "… — composer install: exit status 1",
		},
		{
			name: "a bare exit status never borrows output from an earlier step",
			out:  "→ composer install\nInstalling dependencies\n→ npm run build\n ✗ npm run build: exit status 1\n",
			want: "npm run build: exit status 1",
		},
		{
			name: "empty output yields nothing rather than a stray separator",
			out:  "\n\n",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := failureMessage(c.out); got != c.want {
				t.Errorf("failureMessage() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFailureMessage_trimsAWallOfOutput(t *testing.T) {
	got := failureMessage(" ✗ " + strings.Repeat("x", 500))
	if len([]rune(got)) > 301 {
		t.Errorf("message is %d runes, want it trimmed", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a trimmed message must say so, got %q", got[len(got)-10:])
	}
}
