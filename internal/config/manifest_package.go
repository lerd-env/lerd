package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ManifestPackage names a package a project depends on, in whichever manifest
// its language keeps: package.json, requirements.txt, pyproject.toml, Gemfile
// or Cargo.toml.
type ManifestPackage struct {
	File string `yaml:"file" json:"file"`
	Name string `yaml:"name" json:"name"`
}

// manifestParsers read the direct dependencies of each manifest lerd knows.
var manifestParsers = map[string]func([]byte) map[string]bool{
	"package.json":     parsePackageJSON,
	"requirements.txt": parseRequirements,
	"pyproject.toml":   parsePyproject,
	"Gemfile":          parseGemfile,
	"Cargo.toml":       parseCargo,
}

type manifestCacheEntry struct {
	modTime  time.Time
	size     int64
	packages map[string]bool
}

var (
	manifestCacheMu sync.Mutex
	manifestCache   = map[string]manifestCacheEntry{}

	unknownManifestWarned sync.Map
)

// matches reports whether the manifest in dir lists the package.
func (p ManifestPackage) matches(dir string) bool {
	parse, ok := manifestParsers[p.File]
	if !ok {
		// Comes from a store newer than this binary, so it is refused, not guessed.
		if _, warned := unknownManifestWarned.LoadOrStore(p.File, true); !warned {
			fmt.Fprintf(os.Stderr, "lerd: a framework definition reads packages from %q, which this lerd does not know; update lerd to use it\n", p.File)
		}
		return false
	}
	name := p.Name
	if isPythonManifest(p.File) {
		name = pythonName(name)
	}
	return name != "" && manifestPackages(filepath.Join(dir, p.File), parse)[name]
}

// manifestPackages reuses the last parse while the file's mtime and size hold,
// since detection asks once per rule per site on every dashboard snapshot.
func manifestPackages(path string, parse func([]byte) map[string]bool) map[string]bool {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	manifestCacheMu.Lock()
	defer manifestCacheMu.Unlock()
	if e, ok := manifestCache[path]; ok && e.modTime.Equal(st.ModTime()) && e.size == st.Size() {
		return e.packages
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	packages := parse(data)
	manifestCache[path] = manifestCacheEntry{modTime: st.ModTime(), size: st.Size(), packages: packages}
	return packages
}

func parsePackageJSON(data []byte) map[string]bool {
	var m struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return keysOf(m.Dependencies, m.DevDependencies)
}

func parseCargo(data []byte) map[string]bool {
	var m struct {
		Dependencies    map[string]any `toml:"dependencies"`
		DevDependencies map[string]any `toml:"dev-dependencies"`
	}
	if toml.Unmarshal(data, &m) != nil {
		return nil
	}
	return keysOf(m.Dependencies, m.DevDependencies)
}

func parsePyproject(data []byte) map[string]bool {
	var m struct {
		Project struct {
			Dependencies []string            `toml:"dependencies"`
			Optional     map[string][]string `toml:"optional-dependencies"`
		} `toml:"project"`
		// A group may also include another group, which is a table.
		Groups map[string][]any `toml:"dependency-groups"`
		Tool   struct {
			Poetry struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if toml.Unmarshal(data, &m) != nil {
		return nil
	}
	lines := m.Project.Dependencies
	for _, deps := range m.Project.Optional {
		lines = append(lines, deps...)
	}
	for _, deps := range m.Groups {
		for _, d := range deps {
			if s, ok := d.(string); ok {
				lines = append(lines, s)
			}
		}
	}
	out := pythonPackages(lines)
	for name := range m.Tool.Poetry.Dependencies {
		out[pythonName(name)] = true
	}
	return out
}

func parseRequirements(data []byte) map[string]bool {
	return pythonPackages(strings.Split(string(data), "\n"))
}

var gemRe = regexp.MustCompile(`(?m)^\s*gem[\s(]+["']([^"']+)["']`)

func parseGemfile(data []byte) map[string]bool {
	out := map[string]bool{}
	for _, m := range gemRe.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = true
	}
	return out
}

// pythonReqRe takes the name off a requirement, before extras or a version.
var pythonReqRe = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)`)

func pythonPackages(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines {
		if m := pythonReqRe.FindStringSubmatch(l); m != nil {
			out[pythonName(m[1])] = true
		}
	}
	return out
}

func isPythonManifest(file string) bool {
	return file == "requirements.txt" || file == "pyproject.toml"
}

var pythonSepRe = regexp.MustCompile(`[-_.]+`)

// pythonName normalises a package name the way pip does (PEP 503).
func pythonName(name string) string {
	return strings.ToLower(pythonSepRe.ReplaceAllString(name, "-"))
}

func keysOf(maps ...map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, m := range maps {
		for k := range m {
			out[k] = true
		}
	}
	return out
}
