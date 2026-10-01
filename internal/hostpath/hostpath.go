// Package hostpath maps Windows host paths to the Linux paths the same
// locations have inside the Podman machine, and back. Lerd bind-mounts a site at
// its own path on macOS and Linux; a Windows path such as C:\Sites\app cannot
// exist inside a Linux VM, so the container side needs a translated twin.
package hostpath

import "strings"

// DriveRoot is where the VM exposes the host's drives: /mnt/c for C:. It is a
// variable so the mount layout of a given machine provider can be set in one
// place.
var DriveRoot = "/mnt"

// IsWindowsPath reports whether p starts with a drive letter and separator.
func IsWindowsPath(p string) bool {
	if len(p) < 3 || p[1] != ':' || (p[2] != '\\' && p[2] != '/') {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// ToVM returns the VM path of a Windows host path, and any other path (already
// POSIX, or relative) unchanged.
func ToVM(p string) string {
	if !IsWindowsPath(p) {
		return p
	}
	rest := strings.ReplaceAll(p[3:], `\`, "/")
	rest = strings.TrimRight(rest, "/")
	drive := strings.ToLower(p[:1])
	out := strings.TrimRight(DriveRoot, "/") + "/" + drive
	if rest != "" {
		out += "/" + rest
	}
	return out
}

// FromVM is the inverse of ToVM: a path under a drive mount becomes its Windows
// path, and anything else is returned unchanged.
func FromVM(p string) string {
	prefix := strings.TrimRight(DriveRoot, "/") + "/"
	if !strings.HasPrefix(p, prefix) {
		return p
	}
	rest := p[len(prefix):]
	if rest == "" || (len(rest) > 1 && rest[1] != '/') {
		return p
	}
	c := rest[0] | 0x20
	if c < 'a' || c > 'z' {
		return p
	}
	drive := strings.ToUpper(rest[:1])
	tail := strings.TrimPrefix(rest[1:], "/")
	return drive + `:\` + strings.ReplaceAll(tail, "/", `\`)
}
