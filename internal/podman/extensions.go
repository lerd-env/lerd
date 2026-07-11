package podman

// BundledExtensions returns the set of PHP extensions included in the default lerd FPM image.
func BundledExtensions() []string {
	return []string{
		// always-compiled PHP core
		"ctype", "dom", "fileinfo", "filter", "hash", "iconv",
		"json", "libxml", "openssl", "pcre", "pdo", "phar", "posix",
		"readline", "session", "simplexml", "sodium", "spl", "tokenizer",
		"xml", "xmlreader", "xmlwriter", "zlib",
		// docker-php-ext-install
		"bcmath", "bz2", "calendar", "curl", "dba", "exif", "ftp", "gd", "gmp",
		"intl", "ldap", "mbstring", "mysqli", "opcache", "pcntl",
		"pdo_mysql", "pdo_pgsql", "pdo_sqlite", "soap", "shmop",
		"sockets", "sqlite3", "sysvmsg", "sysvsem", "sysvshm", "xsl", "zip",
		// PECL
		"redis", "imagick", "igbinary", "mongodb", "pcov", "xdebug",
	}
}
