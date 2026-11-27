package config

import (
	"os"
	"sync"
	"time"
)

// storeReadFile is swappable so a test can count reads.
var storeReadFile = os.ReadFile

type storeFileEntry struct {
	modTime time.Time
	size    int64
	value   any
	ok      bool
}

var (
	storeFileMu    sync.Mutex
	storeFileCache = map[string]storeFileEntry{}
)

// loadStoreFile parses a store file once per version of it on disk, since
// framework detection reads the same definitions for every site on every tick.
// The value is shared between callers, so they must treat it as read-only.
func loadStoreFile[T any](path string, parse func([]byte) (T, bool)) (T, bool) {
	var zero T
	st, err := os.Stat(path)
	if err != nil {
		return zero, false
	}
	storeFileMu.Lock()
	defer storeFileMu.Unlock()
	if e, ok := storeFileCache[path]; ok && e.modTime.Equal(st.ModTime()) && e.size == st.Size() {
		v, _ := e.value.(T)
		return v, e.ok
	}
	data, err := storeReadFile(path)
	if err != nil {
		return zero, false
	}
	v, ok := parse(data)
	storeFileCache[path] = storeFileEntry{modTime: st.ModTime(), size: st.Size(), value: v, ok: ok}
	return v, ok
}
