// Package imgledger records the container image refs lerd itself pulled. Podman
// has no way to label an image in place, so the ledger is lerd's provenance
// marker: cleanup reclaims lerd's own catalog leftovers while leaving an image
// the user pulled independently that happens to share a catalog repo untouched.
package imgledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/geodro/lerd/internal/config"
)

var mu sync.Mutex

// pathFn is the seam tests override to redirect the ledger file.
var pathFn = defaultPath

func defaultPath() string {
	return filepath.Join(config.DataDir(), "pulled-images.json")
}

// Record notes that lerd has pulled ref. Best-effort: a write failure only keeps
// cleanup conservative (an unrecorded image is never reaped as lerd's), so the
// pull path ignores the outcome.
func Record(ref string) {
	if ref == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	set := load()
	if set[ref] {
		return
	}
	set[ref] = true
	save(set)
}

// Load returns the set of refs lerd has recorded pulling. A missing or unreadable
// ledger yields an empty set, so cleanup reaps nothing it can't prove is lerd's.
func Load() map[string]bool {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

func load() map[string]bool {
	set := map[string]bool{}
	b, err := os.ReadFile(pathFn())
	if err != nil {
		return set
	}
	var refs []string
	if json.Unmarshal(b, &refs) != nil {
		return set
	}
	for _, r := range refs {
		set[r] = true
	}
	return set
}

// save writes the ledger atomically (temp then rename) so a concurrent reader or
// a mid-write crash can never see a truncated file.
func save(set map[string]bool) {
	refs := make([]string, 0, len(set))
	for r := range set {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	b, err := json.Marshal(refs)
	if err != nil {
		return
	}
	path := pathFn()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
