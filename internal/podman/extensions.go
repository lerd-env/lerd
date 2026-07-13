package podman

import "strings"

// BundledExtensions returns the set of PHP extensions included in the default lerd FPM image.
func BundledExtensions() []string {
	return []string{
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
// BundledExtensions uses, so both spellings resolve to the same extension.
func CanonicalExtension(name string) string {
	name = strings.ToLower(name)
	for install, platform := range composerPlatformNames {
		if platform == name {
			return install
		}
	}
	return name
}
