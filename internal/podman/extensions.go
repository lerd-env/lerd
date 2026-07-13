package podman

import (
	"bufio"
	"fmt"
	"strings"
)

// bundledSince records the first PHP version whose image actually ships an
// extension. ext/random is core only from 8.2, and PECL mongodb no longer builds
// below 8.1, where the Containerfile's tolerant `|| true` drops it silently.
var bundledSince = map[string][2]int{
	"random":  {8, 2},
	"mongodb": {8, 1},
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
		"intl", "ldap", "mbstring", "mysqli", "opcache", "pcntl",
		"pdo_mysql", "pdo_pgsql", "pdo_sqlite", "soap", "shmop",
		"sockets", "sqlite3", "sysvmsg", "sysvsem", "sysvshm", "xsl", "zip",
		// PECL
		"redis", "imagick", "igbinary", "mongodb", "pcov", "xdebug",
	}

	bundled := make([]string, 0, len(all))
	for _, ext := range all {
		if since, gated := bundledSince[ext]; gated && !phpAtLeast(phpVersion, since[0], since[1]) {
			continue
		}
		bundled = append(bundled, ext)
	}
	return bundled
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
