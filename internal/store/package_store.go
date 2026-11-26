package store

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterPackageFetchHook(autoFetchPackage)
}

// autoFetchPackage downloads a package definition and caches it locally. Called
// by the framework resolver when a project requires a package the store index
// lists and the local copy is missing or a day old.
func autoFetchPackage(name, version string) (*config.FrameworkPackage, error) {
	return NewClient().FetchPackage(name, version)
}

// siblingBases returns the store locations of dir, a directory beside the
// framework definitions such as packages or starters, which older binaries never fetch.
func (c *Client) siblingBases(dir string) []string {
	bases := append([]string{c.BaseURL}, c.Fallbacks...)
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		trimmed := strings.TrimSuffix(base, "/")
		if cut, ok := strings.CutSuffix(trimmed, "/frameworks"); ok {
			trimmed = cut
		}
		out = append(out, trimmed+"/"+dir)
	}
	return out
}

// PackageVersions returns the versions of a package to fetch. Every package has
// its unversioned file, the one that serves a project below the first major that
// needed a file of its own, so that file is always in the list.
func PackageVersions(entry config.StorePackageEntry) []string {
	return append([]string{""}, entry.Versions...)
}

// PackageLabel spells a package and version the way the framework store spells
// one of its definitions.
func PackageLabel(name, version string) string {
	if version == "" {
		return name
	}
	return name + "@" + version
}

// FetchPackage downloads one package definition from the store, the layer that
// declares a composer package's workers, commands and doctor checks once
// instead of once per framework major.
func (c *Client) FetchPackage(name, version string) (*config.FrameworkPackage, error) {
	slug := config.PackageSlug(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid package name %q", name)
	}
	if version != "" {
		slug += "@" + version
	}
	data, err := c.fetchFrom(c.siblingBases("packages"), slug+".yaml")
	if err != nil {
		return nil, fmt.Errorf("fetching package %s: %w", name, err)
	}
	var pkg config.FrameworkPackage
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package %s: %w", name, err)
	}
	// The name the store answered with decides where this is cached, and the
	// request already proved it is a name we asked for; a file claiming another
	// package would otherwise overwrite that package's cache.
	if pkg.Package != name {
		return nil, fmt.Errorf("package %s declares itself as %q", name, pkg.Package)
	}
	if pkg.Version != version {
		return nil, fmt.Errorf("package %s@%s declares itself as version %q", name, version, pkg.Version)
	}
	if err := config.SaveStorePackage(&pkg); err != nil {
		return nil, err
	}
	// Marks stay with the definitions: a package's worker draws the same
	// workers/<icon>.svg a framework's does.
	c.fetchWorkerIcons(pkg.Workers)
	return &pkg, nil
}
