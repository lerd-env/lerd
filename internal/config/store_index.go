package config

import (
	"encoding/json"
	"os"
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

// loadCachedStoreEntries reads the locally cached framework store index. Returns
// nil when the cache is absent or unreadable (e.g. a fresh machine that has not
// reached the store yet), so callers fall back to the built-in adapters.
func loadCachedStoreEntries() []cachedStoreEntry {
	data, err := os.ReadFile(StoreIndexFile())
	if err != nil {
		return nil
	}
	var idx struct {
		Frameworks []cachedStoreEntry `json:"frameworks"`
	}
	if json.Unmarshal(data, &idx) != nil {
		return nil
	}
	return idx.Frameworks
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
