package config

import (
	"slices"
	"sort"
	"sync"

	"github.com/spf13/viper"
)

// updateGlobalMu serializes the read-modify-write below within a process. It
// does not (and cannot) coordinate across processes; it exists so the parallel
// per-version builds php:rebuild fires can't interleave their saves.
var updateGlobalMu sync.Mutex

// UpdateGlobal applies fn to a freshly loaded config and saves it.
//
// Mutating a config value that was loaded earlier is a lost update whenever
// anything else wrote in between, and something does: an image build records
// what each version realised (podman.RecordRealisedSet) while the command that
// started it is still holding the copy it loaded minutes ago. Saving that copy
// afterwards silently erased the record. Re-reading here keeps the window to
// the length of fn.
func UpdateGlobal(fn func(*GlobalConfig)) error {
	updateGlobalMu.Lock()
	defer updateGlobalMu.Unlock()
	cfg, err := LoadGlobal()
	if err != nil {
		return err
	}
	fn(cfg)
	return SaveGlobal(cfg)
}

// RealisedPHPSet is what one version's image actually loaded, verified after
// its build. Not every declared entry can be honoured everywhere: mongodb does
// not build below 8.1, and the legacy 7.4/8.0 images are Alpine 3.16, so apk
// names and availability differ. Recording the truth per version is what lets
// lerd warn about a gap instead of advertising what an image does not have.
type RealisedPHPSet struct {
	// Hash is the declared-set fingerprint the image was measured against. It is
	// always set, which is also what keeps a record from serializing as an empty
	// map: a version that loaded nothing of the declared set (mongodb on the
	// Alpine 3.16 8.0 image) records empty Extensions and Packages, and viper
	// drops an empty-map entry on load. Without a leaf the "8.0" key vanishes,
	// MissingFromImage reads it back as "no record", and the whole declared set
	// is then reported as loaded, the false success this record exists to prevent.
	Hash       string   `yaml:"hash" mapstructure:"hash"`
	Extensions []string `yaml:"extensions,omitempty" mapstructure:"extensions"`
	Packages   []string `yaml:"packages,omitempty" mapstructure:"packages"`
}

// migrateUnifiedPHPSets folds the legacy per-version php.extensions and
// php.packages maps into one declared set each.
//
// It runs against raw viper before Unmarshal, because the legacy shape is a
// map and the current one is a list: decoding would fail on an old config
// before any struct-level migration could fix it.
//
// The fold is a union of every version's entries, which over-applies by
// definition (an extension added only to 8.4 starts targeting 7.4 too). That is
// deliberate and safe: the image build is tolerant by design, every custom
// extension and package step ends in `|| true`, and the realised set records
// per version what actually loaded. The alternative, keeping the entries pinned
// to the version they were added to, is the bug being fixed.
//
// It deliberately does not save. LoadGlobal is a hot read path (every command,
// the watcher, the UI), and writing from it would race concurrent processes all
// folding at once. Folding is cheap and deterministic, so every read is correct
// from the first one, and the file itself is rewritten in the unified shape by
// the next SaveGlobal any mutation performs.
func migrateUnifiedPHPSets(v *viper.Viper) {
	for _, key := range []string{"php::extensions", "php::packages"} {
		if unified, ok := foldPerVersionSet(v.Get(key)); ok {
			v.Set(key, unified)
		}
	}
}

// foldPerVersionSet unions a legacy map[version][]entry into a sorted list.
// ok is false for anything already on the unified shape, so a current config
// is never marked dirty. Sorting keeps the result stable across loads: map
// iteration order is random, and an unstable list would rewrite the config and
// re-fingerprint every image on every load.
func foldPerVersionSet(raw any) (unified []string, ok bool) {
	perVersion, isMap := raw.(map[string]any)
	if !isMap || len(perVersion) == 0 {
		return nil, false
	}
	for _, entries := range perVersion {
		list, isList := entries.([]any)
		if !isList {
			continue
		}
		for _, e := range list {
			s, isStr := e.(string)
			if !isStr || s == "" || slices.Contains(unified, s) {
				continue
			}
			unified = append(unified, s)
		}
	}
	sort.Strings(unified)
	return unified, true
}
