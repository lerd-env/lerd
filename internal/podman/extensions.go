package podman

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// bundledSince records the first PHP version whose image actually ships an
// extension. ext/random is core only from 8.2, and PECL mongodb no longer builds
// below 8.1, where the Containerfile's tolerant `|| true` drops it silently.
var bundledSince = map[string][2]int{
	"random":  {8, 2},
	"mongodb": {8, 1},
}

// prereleaseUnbuildable are the third-party extensions whose sources do not yet
// compile against a prerelease PHP, so the tolerant build drops them and nothing
// may advertise a name the image never loads. Shrink this as upstream catches
// up; the base image build fails on anything advertised and missing, so an entry
// left here too long costs a name, never a broken image.
var prereleaseUnbuildable = map[string]bool{
	"igbinary": true,
	"pcov":     true,
	"xdebug":   true,
}

// BundledExtensions returns the PHP extensions the default lerd FPM image ships
// for phpVersion. Version-gated names are left out of the versions that do not
// build them, so no caller advertises an extension the image never loads.
func BundledExtensions(phpVersion string) []string {
	all := []string{
		// always-compiled PHP core
		"ctype", "date", "dom", "fileinfo", "filter", "hash", "iconv",
		"json", "libxml", "mysqlnd", "openssl", "pcre", "pdo", "phar", "posix",
		"random", "readline", "reflection", "session", "simplexml", "sodium",
		"spl", "tokenizer", "xml", "xmlreader", "xmlwriter", "zlib",
		// docker-php-ext-install
		"bcmath", "bz2", "calendar", "curl", "dba", "exif", "ftp", "gd", "gmp",
		"intl", "ldap", "mbstring", "mysqli", "odbc", "opcache", "pcntl",
		"pdo_mysql", "pdo_odbc", "pdo_pgsql", "pdo_sqlite", "soap", "shmop",
		"sockets", "sqlite3", "sysvmsg", "sysvsem", "sysvshm", "xsl", "zip",
		// PECL
		"redis", "imagick", "igbinary", "mongodb", "pcov", "xdebug",
	}

	prerelease := config.IsPrereleasePHPVersion(phpVersion)
	bundled := make([]string, 0, len(all))
	for _, ext := range all {
		if since, gated := bundledSince[ext]; gated && !phpAtLeast(phpVersion, since[0], since[1]) {
			continue
		}
		if prerelease && prereleaseUnbuildable[ext] {
			continue
		}
		bundled = append(bundled, ext)
	}
	return bundled
}

// WithoutBundled drops the extensions the image for phpVersion already ships.
// Rebuilding one as a custom extension layers a bare docker-php-ext-install on
// top of the base image, which loses the configure flags the base build passed
// it: that is how ftp lost FTPS support after #1583 (#1576).
func WithoutBundled(phpVersion string, exts []string) []string {
	bundled := map[string]bool{}
	for _, e := range BundledExtensions(phpVersion) {
		bundled[e] = true
	}
	kept := make([]string, 0, len(exts))
	for _, e := range exts {
		if !bundled[CanonicalExtension(e)] {
			kept = append(kept, e)
		}
	}
	return kept
}

// BundledSince returns the first PHP version that ships ext, for the extensions an
// older image genuinely cannot build. The second result is false for every other
// extension, which is the ones php:ext add can install on request.
func BundledSince(ext string) (string, bool) {
	since, gated := bundledSince[CanonicalExtension(ext)]
	if !gated {
		return "", false
	}
	return fmt.Sprintf("%d.%d", since[0], since[1]), true
}

// phpAtLeast reports whether phpVersion is at least major.minor. A version that
// will not parse is treated as new enough: the gated extensions are the exception,
// and advertising them is the behaviour every supported version already gets.
func phpAtLeast(phpVersion string, wantMajor, wantMinor int) bool {
	major, minor, err := splitMajorMinor(phpVersion)
	if err != nil {
		return true
	}
	return versionAtLeast(major, minor, wantMajor, wantMinor)
}

// composerPlatformNames maps an extension's install name to the name composer's
// platform repository publishes it under. Composer derives ext-* names from the
// module name PHP reports, which for OPcache is "Zend OPcache", never "opcache".
var composerPlatformNames = map[string]string{
	"opcache": "zend-opcache",
}

// ComposerPlatformName returns the ext-* name (without the prefix) that composer
// publishes for a bundled extension. Most extensions are published as-is.
func ComposerPlatformName(ext string) string {
	if name, ok := composerPlatformNames[strings.ToLower(ext)]; ok {
		return name
	}
	return ext
}

// CanonicalExtension folds a composer ext-* name back onto the install name
// BundledExtensions uses, so both spellings resolve to the same extension. The
// space fold also lands `php -m`'s "Zend OPcache" on the same name.
func CanonicalExtension(name string) string {
	name = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
	for install, platform := range composerPlatformNames {
		if platform == name {
			return install
		}
	}
	return name
}

// phpModules folds `php -m` output into the install names BundledExtensions uses:
// the module list prints display names, so PDO, SimpleXML and "Zend OPcache" all
// have to be canonicalised, and the [PHP Modules] section headers skipped.
func phpModules(out string) map[string]bool {
	modules := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		modules[CanonicalExtension(line)] = true
	}
	return modules
}

// MissingBundledExtensions returns every extension BundledExtensions advertises
// for phpVersion that the image's `php -m` output does not report. Only the built
// image can falsify the list, so this is what CI runs against a fresh build.
func MissingBundledExtensions(phpVersion, phpMinusM string) []string {
	modules := phpModules(phpMinusM)
	var missing []string
	for _, ext := range BundledExtensions(phpVersion) {
		if !modules[CanonicalExtension(ext)] {
			missing = append(missing, ext)
		}
	}
	return missing
}
