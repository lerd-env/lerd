package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var publishedPHPVersions = []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}

// TestPublishedImagesReportBundledExtensions is the direction no Containerfile
// parse can reach: a name BundledExtensions advertises that nothing in the image
// loads. Fixtures are real output from the published bases, refreshed after a
// rebuild with: podman run --rm ghcr.io/lerd-env/lerd-php<nn>-fpm-base:<hash> php -m
func TestPublishedImagesReportBundledExtensions(t *testing.T) {
	for _, version := range publishedPHPVersions {
		t.Run(version, func(t *testing.T) {
			modules := readModuleFixture(t, version)
			if missing := MissingBundledExtensions(version, modules); len(missing) > 0 {
				t.Errorf("PHP %s advertises extensions its image never loads: %v", version, missing)
			}
		})
	}
}

// TestImageReportsAdvertisedExtensions is the CI guard: base-images.yml captures
// `php -m` from the image it just built and points these vars at it, so a dropped
// extension fails the build instead of reaching users. Skipped everywhere else.
func TestImageReportsAdvertisedExtensions(t *testing.T) {
	path, version := os.Getenv("LERD_PHP_MODULES"), os.Getenv("LERD_PHP_VERSION")
	if path == "" || version == "" {
		t.Skip("set LERD_PHP_MODULES and LERD_PHP_VERSION to check a built image")
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading php -m output: %v", err)
	}
	if missing := MissingBundledExtensions(version, string(out)); len(missing) > 0 {
		t.Fatalf("PHP %s image does not load advertised extensions: %v", version, missing)
	}
}

// TestPHPModulesFoldsDisplayNames pins the apples-to-apples problem: `php -m`
// prints display names, so PDO, SimpleXML and above all "Zend OPcache" have to
// fold onto install names before anything is compared.
func TestPHPModulesFoldsDisplayNames(t *testing.T) {
	out := "[PHP Modules]\nCore\nPDO\nSimpleXML\nZend OPcache\nxdebug\n\n[Zend Modules]\nXdebug\nZend OPcache\n"
	modules := phpModules(out)
	for _, want := range []string{"pdo", "simplexml", "opcache", "xdebug"} {
		if !modules[want] {
			t.Errorf("phpModules did not fold %q out of %q", want, out)
		}
	}
	if modules["zend opcache"] || modules["Zend OPcache"] {
		t.Error("phpModules kept the raw OPcache display name instead of folding it")
	}
}

// TestMissingBundledExtensionsReportsAbsent guards the guard: an image that drops
// an advertised extension must be reported, or the check silently passes forever.
func TestMissingBundledExtensionsReportsAbsent(t *testing.T) {
	modules := readModuleFixture(t, "8.4")
	stripped := strings.ReplaceAll(modules, "\nftp\n", "\n")

	missing := MissingBundledExtensions("8.4", stripped)
	if len(missing) != 1 || missing[0] != "ftp" {
		t.Fatalf("MissingBundledExtensions = %v, want [ftp]", missing)
	}
}

// TestBundledExtensionsAreVersionGated checks the two extensions the older images
// genuinely do not ship: ext/random is core only from 8.2, and PECL mongodb no
// longer builds for 8.0 and below, where the Containerfile's `|| true` drops it.
func TestBundledExtensionsAreVersionGated(t *testing.T) {
	cases := []struct {
		version string
		ext     string
		want    bool
	}{
		{"8.1", "random", false},
		{"8.2", "random", true},
		{"8.0", "mongodb", false},
		{"8.1", "mongodb", true},
		{"7.4", "curl", true},
	}
	for _, c := range cases {
		got := false
		for _, e := range BundledExtensions(c.version) {
			if e == c.ext {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("BundledExtensions(%q) lists %q = %v, want %v", c.version, c.ext, got, c.want)
		}
	}
}

func readModuleFixture(t *testing.T, version string) string {
	t.Helper()
	out, err := os.ReadFile(filepath.Join("testdata", "php-modules", version+".txt"))
	if err != nil {
		t.Fatalf("reading php -m fixture: %v", err)
	}
	return string(out)
}
