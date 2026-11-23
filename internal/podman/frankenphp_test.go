package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/composer"
)

// Under the native runtime a FrankenPHP site's commands run in its container,
// composer among them, invoked by the phar's host path and needing its home.
// A source that is not on disk yet is left out rather than mounted empty.
func TestFrankenPHPQuadletMountsComposer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	composerHome := t.TempDir()
	t.Setenv("COMPOSER_HOME", composerHome)
	phar := composer.PharPath()

	content, err := GenerateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, phar) {
		t.Errorf("a phar not downloaded yet must not be mounted:\n%s", content)
	}

	if err := os.MkdirAll(filepath.Dir(phar), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phar, []byte("phar"), 0644); err != nil {
		t.Fatal(err)
	}
	content, err = GenerateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Volume=" + phar + ":" + phar + ":ro",
		"Volume=" + composerHome + ":" + composerHome + ":rw",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("quadlet missing %q:\n%s", want, content)
		}
	}
}

func TestFrankenPHPContainerName(t *testing.T) {
	got := FrankenPHPContainerName("myapp")
	if got != "lerd-fp-myapp" {
		t.Fatalf("FrankenPHPContainerName: want lerd-fp-myapp, got %s", got)
	}
}

func TestFrankenPHPImage(t *testing.T) {
	// FrankenPHPImage is now the lerd-derived image; the upstream tag it builds
	// FROM is FrankenPHPBaseImage.
	tests := []struct {
		version, wantDerived, wantBase string
	}{
		{"8.2", "localhost/lerd-frankenphp82:local", "docker.io/dunglas/frankenphp:php8.2-alpine"},
		{"8.4", "localhost/lerd-frankenphp84:local", "docker.io/dunglas/frankenphp:php8.4-alpine"},
		{"8.5", "localhost/lerd-frankenphp85:local", "docker.io/dunglas/frankenphp:php8.5-alpine"},
		{"8.1", "localhost/lerd-frankenphp85:local", "docker.io/dunglas/frankenphp:php8.5-alpine"}, // no frankenphp tag → latest
		{"", "localhost/lerd-frankenphp85:local", "docker.io/dunglas/frankenphp:php8.5-alpine"},
	}
	for _, tt := range tests {
		if got := FrankenPHPImage(tt.version); got != tt.wantDerived {
			t.Errorf("FrankenPHPImage(%q): want %s, got %s", tt.version, tt.wantDerived, got)
		}
		if got := FrankenPHPBaseImage(tt.version); got != tt.wantBase {
			t.Errorf("FrankenPHPBaseImage(%q): want %s, got %s", tt.version, tt.wantBase, got)
		}
	}
}

func TestGenerateFrankenPHPQuadlet(t *testing.T) {
	entry := []string{"php", "artisan", "octane:start", "--server=frankenphp"}
	env := map[string]string{"FRANKENPHP_CONFIG": "worker ./public/index.php"}
	content, err := GenerateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", entry, env)
	if err != nil {
		t.Fatalf("GenerateFrankenPHPQuadlet: %v", err)
	}

	mustContain := []string{
		"ContainerName=lerd-fp-myapp",
		"Image=localhost/lerd-frankenphp84:local",
		"Network=lerd",
		"Volume=/home/user/myapp:/home/user/myapp:rw",
		"--workdir=/home/user/myapp",
		`Environment="FRANKENPHP_CONFIG=worker ./public/index.php"`,
		"Exec=php artisan octane:start --server=frankenphp",
		"Restart=always",
		// Debug tooling parity: the same conf.d inis and bridge dir the FPM
		// container mounts (dump bridge, devtools, xdebug). SPX is excluded.
		"/usr/local/etc/php/conf.d/97-lerd-dump.ini:ro",
		"/usr/local/etc/php/conf.d/96-lerd-devtools.ini:ro",
		"/usr/local/etc/php/conf.d/99-xdebug.ini:ro",
		"/usr/local/etc/php/conf.d/98-lerd-user.ini:ro",
		"/usr/local/etc/php/conf.d/95-lerd-shared.ini:ro",
		":/usr/local/etc/lerd:ro",
	}
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("generated quadlet missing %q\n%s", s, content)
		}
	}
	// SPX is not wired for FrankenPHP (can't profile Octane workers).
	if strings.Contains(content, "spx") || strings.Contains(content, "/var/spx") {
		t.Errorf("SPX should not be mounted in the FrankenPHP quadlet:\n%s", content)
	}
}

func TestShellJoinQuotesWhitespace(t *testing.T) {
	got := shellJoin([]string{"frankenphp", "run", "--with spaces", `has"quote`})
	want := `frankenphp run "--with spaces" "has\"quote"`
	if got != want {
		t.Fatalf("shellJoin:\n  want %s\n  got  %s", want, got)
	}
}

func TestShellJoinEscapesBackslash(t *testing.T) {
	// An arg ending in a backslash must escape it so the closing quote isn't
	// swallowed by the parser on the other side.
	got := shellJoin([]string{"cmd", `path\`})
	want := `cmd "path\\"`
	if got != want {
		t.Fatalf("shellJoin backslash:\n  want %s\n  got  %s", want, got)
	}
}
