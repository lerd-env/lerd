package cli

// packagedPrefixes are where a deb, rpm or pacman package puts lerd. /var/usrlocal
// is what /usr/local resolves to on ostree systems (Silverblue), where the
// symlink resolution of the binary's path hides the /usr prefix.
var packagedPrefixes = []string{"/usr/", "/var/usrlocal/"}

func rollbackSupported() error { return nil }
