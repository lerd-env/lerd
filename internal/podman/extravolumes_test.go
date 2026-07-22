package podman

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// bindMountable is the shared guard every Volume=path:path line goes through. It
// must refuse the filesystem root (issue #884) and empty/relative paths, and
// accept real absolute paths.
func TestBindMountable(t *testing.T) {
	refused := []string{"", "/", "//", "/.", "/..", "rel/path", "."}
	for _, p := range refused {
		if bindMountable(p) {
			t.Errorf("bindMountable(%q) = true, want false", p)
		}
	}
	accepted := []string{"/var/www/app", "/home/user/Lerd/site", "/srv/x"}
	for _, p := range accepted {
		if !bindMountable(p) {
			t.Errorf("bindMountable(%q) = false, want true", p)
		}
	}
}

// extraVolumePaths must never return the filesystem root. Mounting /:/:rw over a
// container's own rootfs shadows its entrypoint and shell, so crun aborts with
// exit 127 and nginx/php-fpm never start (issue #884). A "/" among the candidates
// must be dropped, and it must not swallow the other paths through the ancestor
// reduction either.
func TestExtraVolumePaths_refusesFilesystemRoot(t *testing.T) {
	home := "/home/user"

	cases := []struct {
		name       string
		candidates []string
		want       []string
	}{
		{"root alone", []string{"/"}, nil},
		{"root cleaned", []string{"//", "/."}, nil},
		{"root does not swallow siblings", []string{"/", "/opt/site"}, []string{"/opt/site"}},
		{"legit out-of-home path kept", []string{"/var/www/app"}, []string{"/var/www/app"}},
		{"home and under-home dropped", []string{home, home + "/proj"}, nil},
		{"ancestor reduction still applies", []string{"/var/www", "/var/www/app"}, []string{"/var/www"}},
		{"empty and relative dropped", []string{"", "rel/path", "/srv/x"}, []string{"/srv/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extraVolumePaths(tc.candidates, home)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extraVolumePaths(%v) = %v, want %v", tc.candidates, got, tc.want)
			}
		})
	}
}

// A site directory that has gone missing (a branch checkout that removed it, a
// deleted project) must not reach a Volume= line: podman aborts the container
// start with statfs and nginx takes every other site down with it (#1083).
func TestExtraVolumePaths_dropsMissingPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	outside := t.TempDir()
	gone := filepath.Join(outside, "removed-by-checkout")
	reg := config.SiteRegistry{Sites: []config.Site{
		{Name: "present", Path: outside},
		{Name: "gone", Path: gone},
	}}
	if err := config.SaveSites(&reg); err != nil {
		t.Fatal(err)
	}

	got := ExtraVolumePaths()

	if !reflect.DeepEqual(got, []string{outside}) {
		t.Errorf("ExtraVolumePaths() = %v, want [%s]", got, outside)
	}
}

// InjectExtraVolumes is the function that formats every Volume=path:path line, so
// the guard lives there rather than in each caller: an unguarded path reaching it
// must be dropped, while the legitimate out-of-home mounts still go in (#884).
func TestInjectExtraVolumes_refusesUnmountablePaths(t *testing.T) {
	content := "[Container]\nVolume=%h:%h:rw\n"

	got := InjectExtraVolumes(content, []string{"/", "", "rel/path", "/var/www/app"})

	if strings.Contains(got, "Volume=/:/:") {
		t.Errorf("InjectExtraVolumes emitted a root mount:\n%s", got)
	}
	if strings.Contains(got, "Volume=::") || strings.Contains(got, "Volume=rel/path:") {
		t.Errorf("InjectExtraVolumes emitted an empty or relative mount:\n%s", got)
	}
	if !strings.Contains(got, "Volume=/var/www/app:/var/www/app:rw") {
		t.Errorf("InjectExtraVolumes dropped a legitimate out-of-home mount:\n%s", got)
	}
}

// A quadlet with no mount for the project and a --workdir pointing at it is a
// half-configured container. When the path cannot be bind-mounted, the generators
// must refuse to render rather than emit one (#884).
func TestGenerators_refuseUnmountableProjectPath(t *testing.T) {
	for _, path := range []string{"/", "//", "", "rel/path"} {
		if _, err := GenerateCustomContainerQuadlet("app", path, 3000); err == nil {
			t.Errorf("GenerateCustomContainerQuadlet(%q) should have been refused", path)
		}
		if _, err := GenerateFrankenPHPQuadlet("app", path, "8.4", nil, nil); err == nil {
			t.Errorf("GenerateFrankenPHPQuadlet(%q) should have been refused", path)
		}
	}

	if _, err := GenerateCustomContainerQuadlet("app", "/var/www/app", 3000); err != nil {
		t.Errorf("GenerateCustomContainerQuadlet with a real path should succeed: %v", err)
	}
	if _, err := GenerateFrankenPHPQuadlet("app", "/var/www/app", "8.4", nil, nil); err != nil {
		t.Errorf("GenerateFrankenPHPQuadlet with a real path should succeed: %v", err)
	}
}
