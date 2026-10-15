package plugins

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skeleton is the boilerplate plugin `lerd plugin new` copies. Its scripts read
// their name from LERD_PLUGIN_NAME; every other file says hello and is renamed.
//
//go:embed all:skeleton
var skeleton embed.FS

// Scaffold writes a new plugin called name into dest, which must not exist.
func Scaffold(name, dest string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q: use lowercase letters, digits and dashes", name)
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	return fs.WalkDir(skeleton, "skeleton", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, strings.TrimPrefix(path, "skeleton"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := skeleton.ReadFile(path)
		if err != nil {
			return err
		}
		// embed drops file modes, so anything under bin/ is the executable part.
		mode := os.FileMode(0o755)
		if !strings.HasPrefix(path, "skeleton/bin/") {
			mode = 0o644
			data = []byte(renameSkeleton(string(data), name))
		}
		if path == "skeleton/"+ManifestFile {
			data = []byte(strings.Replace(string(data), "\nname: "+name+"\n", "\nname: "+name+"\n"+requiresLine(), 1))
		}
		return os.WriteFile(target, data, mode)
	})
}

// renameSkeleton swaps the skeleton's name for the new one, as an id (hello) and
// as a title (Hello), so "demo-tool" titles its page "Demo tool".
func renameSkeleton(text, name string) string {
	title := strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "-", " ")
	return strings.ReplaceAll(strings.ReplaceAll(text, "hello", name), "Hello", title)
}

// requiresLine pins a new plugin to the lerd release that wrote it, the oldest
// it is known to run on; an untagged build has no release to name.
func requiresLine() string {
	release := releaseRe.FindString(runningVersion)
	if release == "" {
		return ""
	}
	return fmt.Sprintf("lerd: \">=%s\"\n", strings.TrimPrefix(release, "v"))
}
