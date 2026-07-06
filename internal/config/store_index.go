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
