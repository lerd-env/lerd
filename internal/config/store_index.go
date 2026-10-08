package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// cachedStoreEntry mirrors the framework store index fields that offline
// detection needs. The config package cannot import the store package (store
// imports config), so it reads the store-maintained cache file directly. The
// store package writes and refreshes StoreIndexFile(); this is the read side.
type cachedStoreEntry struct {
	Name     string          `json:"name"`
	Label    string          `json:"label"`
	Versions []string        `json:"versions"`
	Latest   string          `json:"latest"`
	Detect   []FrameworkRule `json:"detect"`
}

// StorePackageEntry is one composer package the store publishes declarations
// for. Versions lists the package's own majors that have a file of their own;
// a package whose declarations hold across every major publishes none and is
// served by a single unversioned file.
type StorePackageEntry struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions,omitempty"`
	Latest   string   `json:"latest,omitempty"`
}

// Package types a package definition may declare: composer, the default a
// file without a type has, and npm.
const (
	PackageComposer = "composer"
	PackageNPM      = "npm"
)

// cachedStoreIndex mirrors the store index fields the config package reads.
type cachedStoreIndex struct {
	Frameworks []cachedStoreEntry  `json:"frameworks"`
	Packages   []StorePackageEntry `json:"packages"`
	// NPMPackages are listed apart from Packages, which a lerd that predates
	// them fetches whole on update and would refuse an npm name from.
	NPMPackages []StorePackageEntry `json:"npm_packages"`
}

// loadCachedStoreIndex reads the locally cached framework store index. Returns
// nil when the cache is absent or unreadable (e.g. a fresh machine that has not
// reached the store yet), so callers fall back to the built-in adapters.
func loadCachedStoreIndex() *cachedStoreIndex {
	data, err := os.ReadFile(StoreIndexFile())
	if err != nil {
		return nil
	}
	var idx cachedStoreIndex
	if json.Unmarshal(data, &idx) != nil {
		return nil
	}
	return &idx
}

func loadCachedStoreEntries() []cachedStoreEntry {
	idx := loadCachedStoreIndex()
	if idx == nil {
		return nil
	}
	return idx.Frameworks
}

// cachedStorePackages returns the composer packages the store publishes a
// definition for. Only these are ever looked up for a project, so a project's
// own dependency list is never turned into a fetch for a file the store does
// not have.
func cachedStorePackages() []StorePackageEntry {
	idx := loadCachedStoreIndex()
	if idx == nil {
		return nil
	}
	return idx.Packages
}

// cachedStoreNPMPackages returns the npm packages the store publishes a
// definition for.
func cachedStoreNPMPackages() []StorePackageEntry {
	idx := loadCachedStoreIndex()
	if idx == nil {
		return nil
	}
	return idx.NPMPackages
}

var warnedPackageTypes sync.Map

// packageTypeIs reports whether a package definition is of the type its index
// list says; a type lerd does not know is reported once and never read as one.
func packageTypeIs(pkg *FrameworkPackage, want string) bool {
	got := pkg.Type
	if got == "" {
		got = PackageComposer
	}
	if got != PackageComposer && got != PackageNPM {
		if _, seen := warnedPackageTypes.LoadOrStore(pkg.Package, true); !seen {
			fmt.Fprintf(os.Stderr, "lerd: store package %s has type %q, which this lerd does not know; update lerd to use it\n", pkg.Package, pkg.Type)
		}
		return false
	}
	return got == want
}

// projectOwnsFramework reports whether a framework name belongs to the projects
// that use it rather than to the published store. A project carrying its own
// definition for its own framework is the source of truth for it, so the copy in
// .lerd.yaml is installed and kept current.
//
// A store-published name (laravel, symfony, …) is not: the store owns it and
// every project on the machine shares it. Letting one project's embedded copy
// replace it would rewrite a machine-wide definition from a single repository,
// silently, on nothing more than a detection call. When that copy omitted the
// detect rules the store entry carried, detection then failed for every project
// of that framework. Resolving a difference against a published definition is
// what the link flow's conflict prompt is for.
func projectOwnsFramework(name string) bool {
	return cachedStoreEntryByName(name) == nil
}

// cachedStoreEntryByName returns the cached index entry for name, or nil.
func cachedStoreEntryByName(name string) *cachedStoreEntry {
	entries := loadCachedStoreEntries()
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}
