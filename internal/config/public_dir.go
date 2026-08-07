package config

import (
	"os"
	"path/filepath"
)

// PublicDirFor returns the document root a site is served from, relative to its
// project root. The project's own .lerd.yaml wins, since that is where a project
// states where it serves from and the registry only caches a copy of it. What
// the site records comes next, then the framework definition's, then the
// long-standing "public" default. Every candidate runs through ValidatePublicDir
// on the way, so a hostile value cannot pivot the root out of the project.
//
// The recorded root gives way to the definition's in one case: when it holds no
// index.php and the definition's does. A root lerd guessed is only ever as good
// as the moment it was guessed in, and a project linked before its dependencies
// were installed has an empty document root to walk, so the guess lands on the
// project root and stays there long after the real one has appeared.
func PublicDirFor(site Site) string {
	if proj, err := LoadProjectConfig(site.Path); err == nil {
		if d := validPublicDir(proj.PublicDir); d != "" {
			return d
		}
	}

	recorded := validPublicDir(site.PublicDir)
	declared := ""
	name := site.Framework
	if name == "" {
		name, _ = DetectFrameworkForDir(site.Path)
	}
	if fw, ok := GetFrameworkForDir(name, site.Path); ok {
		declared = validPublicDir(fw.PublicDir)
	}

	if recorded != "" {
		if declared == "" || servesPHPIndex(site.Path, recorded) || !servesPHPIndex(site.Path, declared) {
			return recorded
		}
	}
	if declared != "" {
		return declared
	}
	return "public"
}

// validPublicDir returns dir when it is a safe relative document root, or empty
// so the caller falls through to the next candidate.
func validPublicDir(dir string) string {
	if dir == "" || ValidatePublicDir(dir) != nil {
		return ""
	}
	return dir
}

// servesPHPIndex reports whether a candidate document root holds the index.php
// that makes it one, the same marker DetectPublicDir accepts a directory on.
func servesPHPIndex(sitePath, publicDir string) bool {
	_, err := os.Stat(filepath.Join(sitePath, publicDir, "index.php"))
	return err == nil
}
