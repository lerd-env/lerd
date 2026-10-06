//go:build !darwin

package editor

import "strings"

// installedOffPath finds an editor not on PATH by a desktop entry claiming
// its URL scheme, which is how JetBrains Toolbox and Flatpak install them.
func (e Editor) installedOffPath() bool {
	return schemeHandled(e.url[:strings.Index(e.url, ":")])
}

// dirCommandOffPath has nothing to run: a desktop entry only claims the URL
// scheme, which addresses a file and cannot open a folder.
func (e Editor) dirCommandOffPath(string) []string { return nil }
