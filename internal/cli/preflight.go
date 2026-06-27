package cli

import "strings"

// hasSubIDRange reports whether the /etc/subuid or /etc/subgid content grants
// a range to the given username or uid. Lines are "owner:start:count"; blanks
// and #comments are ignored. Rootless podman needs such a range to build
// images, and its absence shows up only as an opaque tar "Operation not
// permitted" failure (#636).
func hasSubIDRange(content, username, uid string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		owner, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if owner == username || (uid != "" && owner == uid) {
			return true
		}
	}
	return false
}
