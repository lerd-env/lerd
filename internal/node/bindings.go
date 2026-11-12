package node

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/download"
)

// ContainerBinding is a linux-musl package a lockfile names for the PHP
// container's platform, at Path relative to the project.
type ContainerBinding struct {
	Path      string
	Resolved  string
	Integrity string
}

// FetchContainerBindings installs the linux-musl packages dir's lockfile names
// but a host npm install left out, at their lockfile paths, so Node in the PHP
// container loads the native build each requirer locked. Host Node ignores them.
func FetchContainerBindings(ctx context.Context, dir string, w io.Writer) error {
	arch, err := muslArch(runtime.GOARCH)
	if err != nil {
		return nil // no musl Node for this platform, so nothing would load them
	}
	return fetchContainerBindings(ctx, dir, arch, w)
}

func fetchContainerBindings(ctx context.Context, dir, arch string, w io.Writer) error {
	missing := missingContainerBindings(dir, arch)
	if len(missing) == 0 {
		return nil
	}
	fmt.Fprintf(w, "Fetching %d linux-musl package(s) node_modules lacks for Node in the PHP container\n", len(missing))
	for _, b := range missing {
		if err := fetchContainerBinding(ctx, dir, b); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimPrefix(b.Path, "node_modules/"), err)
		}
	}
	return nil
}

func fetchContainerBinding(ctx context.Context, dir string, b ContainerBinding) error {
	want, ok := strings.CutPrefix(b.Integrity, "sha512-")
	if b.Resolved == "" || !ok {
		return fmt.Errorf("package-lock.json gives no tarball with a sha512 integrity to fetch it from")
	}
	target := filepath.Join(dir, b.Path)
	// The lockfile names both the tarball and its hash, so it must not be able
	// to steer the write out of the project through a symlinked folder.
	if !resolvesInside(dir, target) {
		return fmt.Errorf("%s resolves outside the project", b.Path)
	}
	// Staged inside node_modules so the final rename never crosses a filesystem.
	tmp, err := os.MkdirTemp(filepath.Join(dir, "node_modules"), ".lerd-musl-*.tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, "package.tgz")
	if err := download.File(ctx, b.Resolved, archive, 0o644, io.Discard); err != nil {
		return err
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	if sum := sha512.Sum512(data); base64.StdEncoding.EncodeToString(sum[:]) != want {
		return fmt.Errorf("%s does not match the integrity package-lock.json records", b.Resolved)
	}
	tree := filepath.Join(tmp, "tree")
	if err := extractTree(archive, tree); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.Rename(tree, target)
}

// missingContainerBindings reads package-lock.json rather than node_modules:
// npm skips a platform package that does not fit, so only the lockfile still
// lists the linux-musl build the container needs.
func missingContainerBindings(dir, arch string) []ContainerBinding {
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err != nil {
		return nil
	}
	var lock struct {
		Packages map[string]struct {
			OS        []string `json:"os"`
			CPU       []string `json:"cpu"`
			Libc      []string `json:"libc"`
			Resolved  string   `json:"resolved"`
			Integrity string   `json:"integrity"`
		} `json:"packages"`
	}
	if json.Unmarshal(data, &lock) != nil {
		return nil
	}
	var missing []ContainerBinding
	for path, p := range lock.Packages {
		if len(p.OS) == 0 && len(p.CPU) == 0 && len(p.Libc) == 0 {
			continue
		}
		if !allows(p.OS, "linux") || !allows(p.CPU, arch) || !allows(p.Libc, "musl") {
			continue
		}
		if !strings.HasPrefix(path, "node_modules/") || strings.Contains(path, "..") {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			continue
		}
		missing = append(missing, ContainerBinding{Path: path, Resolved: p.Resolved, Integrity: p.Integrity})
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Path < missing[j].Path })
	return missing
}

// resolvesInside reports whether path's deepest existing folder, symlinks
// followed, is dir or below it.
func resolvesInside(dir, path string) bool {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for p := path; ; p = filepath.Dir(p) {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return real == root || strings.HasPrefix(real, root+string(os.PathSeparator))
		}
		if p == filepath.Dir(p) {
			return false
		}
	}
}

// NpmProjectDir is the nearest folder from cwd up holding a package-lock.json,
// the npm project a command run in cwd uses; "" when there is none.
func NpmProjectDir(cwd string) string {
	for d := cwd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "package-lock.json")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			return ""
		}
	}
}

// allows applies npm's os/cpu/libc rule: an empty list allows everything.
func allows(list []string, v string) bool {
	return len(list) == 0 || slices.Contains(list, v)
}
