package composer

import (
	"os"
	"path/filepath"
	"strings"
)

// HomeDir resolves composer's own home the way composer does, so its global
// auth.json and packages are the ones lerd's composer sees. It is a composer
// project like any other: what `composer global require` installed lives here.
func HomeDir() string {
	home, _ := os.UserHomeDir()
	return resolveHome(os.Environ(), home, func(p string) bool {
		info, err := os.Stat(p)
		return err == nil && info.IsDir()
	})
}

// resolveHome mirrors composer's Factory::getHomeDir: COMPOSER_HOME when set,
// else the first existing of the XDG directory (only on a system that uses
// XDG) and ~/.composer, else the first of those candidates.
func resolveHome(environ []string, home string, isDir func(string) bool) string {
	get := func(key string) string {
		for _, e := range environ {
			if k, v, ok := strings.Cut(e, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	if v := get("COMPOSER_HOME"); v != "" {
		return v
	}
	var dirs []string
	if usesXDG(environ, isDir) {
		cfg := get("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		dirs = append(dirs, filepath.Join(cfg, "composer"))
	}
	dirs = append(dirs, filepath.Join(home, ".composer"))
	for _, d := range dirs {
		if isDir(d) {
			return d
		}
	}
	return dirs[0]
}

// usesXDG is composer's own test: any XDG_ variable set, or /etc/xdg on disk.
func usesXDG(environ []string, isDir func(string) bool) bool {
	for _, e := range environ {
		if strings.HasPrefix(e, "XDG_") {
			return true
		}
	}
	return isDir("/etc/xdg")
}
