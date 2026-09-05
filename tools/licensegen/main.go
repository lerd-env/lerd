// Command licensegen regenerates the third-party license notices that ship
// inside the lerd binaries. Run it with `make licenses`.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// pkg is one dependency and the license text that has to travel with it.
type pkg struct {
	Name    string
	Version string
	License string
	Text    string
}

// buildTarget is a binary whose linked dependencies must be disclosed. The
// build tags matter: `make build` compiles cmd/lerd without cgo and with the
// nogui tag, so listing it any other way discloses packages that never ship.
type buildTarget struct {
	pkgPath string
	tags    string
	cgo     string
}

var targets = []buildTarget{
	{pkgPath: "./cmd/lerd", tags: "nogui", cgo: "0"},
	{pkgPath: "./cmd/lerd", tags: "", cgo: "1"},
	{pkgPath: "./cmd/lerd-tray", tags: "", cgo: "1"},
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	goPkgs, err := goModules(root, targets)
	if err != nil {
		fail(err)
	}
	npmPkgs, err := npmPackages(filepath.Join(root, "internal", "ui", "web", "node_modules"))
	if err != nil {
		fail(err)
	}
	out := filepath.Join(root, "internal", "licenses", "THIRD-PARTY-LICENSES.md")
	if err := os.WriteFile(out, []byte(render(goPkgs, npmPkgs)), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d Go modules, %d npm packages)\n", out, len(goPkgs), len(npmPkgs))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "licensegen:", err)
	os.Exit(1)
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("locating the repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// goListPkg mirrors the subset of `go list -json` output we read.
type goListPkg struct {
	Standard bool
	Module   *struct {
		Path    string
		Version string
		Dir     string
		Main    bool
	}
}

// goModules collects every module linked into any of the given build targets.
func goModules(root string, targets []buildTarget) ([]pkg, error) {
	dirs := map[string]pkg{}
	for _, t := range targets {
		args := []string{"list", "-deps", "-json"}
		if t.tags != "" {
			args = append(args, "-tags", t.tags)
		}
		args = append(args, t.pkgPath)
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED="+t.cgo)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list %s: %w: %s", t.pkgPath, err, stderr.String())
		}
		dec := json.NewDecoder(bytes.NewReader(out))
		for {
			var p goListPkg
			if err := dec.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("decoding go list output: %w", err)
			}
			if p.Standard || p.Module == nil || p.Module.Main || p.Module.Dir == "" {
				continue
			}
			if _, seen := dirs[p.Module.Dir]; seen {
				continue
			}
			text, err := licenseText(p.Module.Dir)
			if err != nil {
				return nil, fmt.Errorf("module %s: %w", p.Module.Path, err)
			}
			dirs[p.Module.Dir] = pkg{
				Name:    p.Module.Path,
				Version: p.Module.Version,
				License: detectLicense(text),
				Text:    text,
			}
		}
	}
	return sorted(dirs), nil
}

// npmPackage mirrors the subset of a package.json we read.
type npmPackage struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	License json.RawMessage `json:"license"`
}

// npmPackages collects every installed npm package. The web UI build pulls
// code from both dependency sets (the Svelte runtime is a devDependency yet
// ends up in the bundle), so the whole tree is disclosed rather than guessed at.
func npmPackages(nodeModules string) ([]pkg, error) {
	if _, err := os.Stat(nodeModules); err != nil {
		return nil, fmt.Errorf("%s is missing, run `make install-ui-deps` first", nodeModules)
	}
	found := map[string]pkg{}
	err := filepath.WalkDir(nodeModules, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "package.json" {
			return err
		}
		dir := filepath.Dir(path)
		if dir == nodeModules {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var p npmPackage
		if json.Unmarshal(raw, &p) != nil || p.Name == "" || p.Version == "" {
			return nil
		}
		key := p.Name + "@" + p.Version
		if _, seen := found[key]; seen {
			return nil
		}
		text, err := licenseText(dir)
		if err != nil {
			return fmt.Errorf("package %s: %w", key, err)
		}
		license := declaredLicense(p.License)
		if license == "" {
			license = detectLicense(text)
		}
		found[key] = pkg{Name: p.Name, Version: p.Version, License: license, Text: text}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sorted(found), nil
}

// declaredLicense reads npm's `license` field, which is a string in modern
// packages and an object in a handful of very old ones.
func declaredLicense(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct{ Type string }
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Type
	}
	return ""
}

func sorted(m map[string]pkg) []pkg {
	out := make([]pkg, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// licenseNames matches LICENSE, LICENCE, COPYING and NOTICE in the spellings
// dependencies actually use, including suffixed forms like LICENSE-BSD.
var licenseNames = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice)([-._].*)?$`)

// licenseText concatenates every license and notice file in a package
// directory. An empty result is not an error: a few packages carry their terms
// in the readme only, and the summary marks those as unknown.
func licenseText(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && licenseNames.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(string(raw)))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// detectLicense identifies a license from its text. Order matters: the GNU
// family shares wording, and the BSD variants differ by a single clause.
func detectLicense(text string) string {
	t := strings.ToLower(text)
	switch {
	case t == "":
		return "Unknown"
	case strings.Contains(t, "gnu affero general public"):
		return "AGPL"
	case strings.Contains(t, "gnu lesser general public"):
		return "LGPL"
	case strings.Contains(t, "gnu general public"):
		return "GPL"
	case strings.Contains(t, "mozilla public license"):
		return "MPL-2.0"
	case strings.Contains(t, "apache license"):
		return "Apache-2.0"
	case strings.Contains(t, "free and unencumbered software released into the public domain"):
		return "Unlicense"
	case strings.Contains(t, "cc0 1.0"):
		return "CC0-1.0"
	case strings.Contains(t, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "permission to use, copy, modify, and/or distribute"):
		return "ISC"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		if strings.Contains(t, "neither the name") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	}
	return "Unknown"
}

const header = `# Third-party licenses

lerd is MIT licensed. The binaries it ships also carry code from the projects
listed below, and this file reproduces their copyright notices and license
terms as those licenses require. It is generated by ` + "`make licenses`" + ` from the
real dependency graph, so do not edit it by hand.

Tools lerd downloads onto your machine at runtime, Composer, mkcert, fnm,
Starship, static-php-cli and the PHP language server, are not redistributed by
lerd and keep their own licenses where they are installed. The published
container images are a separate distribution with their own notices.

`

// render writes the notices file: a summary table per ecosystem, then the full
// texts with packages that share an identical text grouped under one copy.
func render(goPkgs, npmPkgs []pkg) string {
	var b strings.Builder
	b.WriteString(header)
	section(&b, "Go modules linked into the lerd binaries", goPkgs)
	section(&b, "npm packages used to build the embedded web UI", npmPkgs)
	return b.String()
}

func section(b *strings.Builder, title string, pkgs []pkg) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(pkgs) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	b.WriteString("| Package | Version | License |\n| --- | --- | --- |\n")
	for _, p := range pkgs {
		fmt.Fprintf(b, "| %s | %s | %s |\n", p.Name, p.Version, p.License)
	}
	b.WriteString("\n")
	for _, g := range groupByText(pkgs) {
		var names []string
		for _, p := range g {
			names = append(names, p.Name+" "+p.Version)
		}
		fmt.Fprintf(b, "### %s\n\n%s\n\n", strings.Join(names, ", "), g[0].License)
		if g[0].Text == "" {
			b.WriteString("No license file is distributed with this package.\n\n")
			continue
		}
		fmt.Fprintf(b, "```\n%s```\n\n", g[0].Text)
	}
}

// groupByText collapses packages that ship byte-identical license texts, which
// is common across families like golang.org/x and keeps the file readable.
func groupByText(pkgs []pkg) [][]pkg {
	var order []string
	groups := map[string][]pkg{}
	for _, p := range pkgs {
		key := p.License + "\x00" + p.Text
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], p)
	}
	out := make([][]pkg, 0, len(order))
	for _, key := range order {
		out = append(out, groups[key])
	}
	return out
}
