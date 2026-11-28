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
// The home is mounted even before it exists, so global packages land on the host.
func TestFrankenPHPQuadletMountsComposer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	composerHome := filepath.Join(t.TempDir(), "not-created-yet")
	t.Setenv("COMPOSER_HOME", composerHome)
	phar := composer.PharPath()

	content, err := GenerateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, phar) {
		t.Errorf("a phar not downloaded yet must not be mounted:\n%s", content)
	}
	if !strings.Contains(content, "Volume="+composerHome+":"+composerHome+":rw") {
		t.Errorf("composer's home must be mounted before it exists:\n%s", content)
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

// A worktree beside the project is outside its mount, so the container mounts
// it too; one inside the project is already covered and is not mounted twice.
func TestFrankenPHPQuadletMountsSiblingWorktrees(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "app")
	sibling := filepath.Join(root, "app-feature")
	inside := filepath.Join(main, ".worktrees", "fix")
	for name, wt := range map[string]string{"feature": sibling, "fix": inside} {
		meta := filepath.Join(main, ".git", "worktrees", name)
		for _, dir := range []string{meta, wt} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(meta, "gitdir"), []byte(wt+"/.git\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	content, err := GenerateFrankenPHPQuadlet("app", main, "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "Volume="+sibling+":"+sibling+":rw") {
		t.Errorf("sibling worktree not mounted:\n%s", content)
	}
	if strings.Contains(content, "Volume="+inside) {
		t.Errorf("a worktree inside the project needs no mount of its own:\n%s", content)
	}

	// A removed worktree must drop out: a mount whose source is gone fails the
	// container's next start, main site included.
	if err := os.RemoveAll(sibling); err != nil {
		t.Fatal(err)
	}
	content, err = GenerateFrankenPHPQuadlet("app", main, "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "Volume="+sibling) {
		t.Errorf("a removed worktree is still mounted:\n%s", content)
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

// The container is published on loopback only when a port was claimed for it.
func TestGenerateFrankenPHPQuadlet_publishesTheHostPort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	with, err := generateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", nil, nil, 18002)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, "PublishPort=127.0.0.1:18002:8000\n") {
		t.Errorf("quadlet does not publish the loopback port:\n%s", with)
	}
	without, err := GenerateFrankenPHPQuadlet("myapp", "/home/user/myapp", "8.4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(without, "PublishPort=") {
		t.Errorf("quadlet published a port nobody claimed:\n%s", without)
	}
}
