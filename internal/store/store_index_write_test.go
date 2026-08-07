package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// indexBytes builds an index big enough that a truncated write lands mid-file.
func indexBytes(label string, entries int) []byte {
	var idx Index
	for i := 0; i < entries; i++ {
		idx.Frameworks = append(idx.Frameworks, IndexEntry{
			Name:     label + "-" + strconv.Itoa(i),
			Label:    label + " " + strings.Repeat("padding ", 16),
			Versions: []string{"1", "2", "3"},
			Latest:   "3",
		})
	}
	data, err := json.Marshal(idx)
	if err != nil {
		panic(err)
	}
	return data
}

// Two fetches racing on the cache used to share one fixed temp name, so both
// truncated and filled the same file and the rename published their blend.
func TestWriteCachedIndex_concurrentWritersNeverPublishAMerge(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	labels := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
	var wg sync.WaitGroup
	for _, label := range labels {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			data := indexBytes(label, 2000)
			for i := 0; i < 5; i++ {
				writeCachedIndex(data)
			}
		}(label)
	}
	wg.Wait()

	idx, err := loadCachedIndex()
	if err != nil {
		t.Fatalf("published index does not parse: %v", err)
	}
	if len(idx.Frameworks) != 2000 {
		t.Fatalf("published index has %d entries, want 2000", len(idx.Frameworks))
	}
	prefix := strings.SplitN(idx.Frameworks[0].Name, "-", 2)[0]
	for _, e := range idx.Frameworks {
		if !strings.HasPrefix(e.Name, prefix+"-") {
			t.Fatalf("published index blends writers: found %q alongside %q", e.Name, prefix)
		}
	}
}

// A reader hitting the cache mid-refresh must get the previous index or the
// complete new one, never a truncated file that fails to unmarshal.
func TestWriteCachedIndex_readerNeverSeesAPartialIndex(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	writeCachedIndex(indexBytes("seed", 2000))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	var writers sync.WaitGroup
	for w := 0; w < 4; w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for i := 0; i < 40; i++ {
				writeCachedIndex(indexBytes("w"+strconv.Itoa(w)+"r"+strconv.Itoa(i), 2000))
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		writers.Wait()
		close(stop)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			idx, err := loadCachedIndex()
			if err != nil {
				t.Errorf("reader saw an unparseable index: %v", err)
				return
			}
			if len(idx.Frameworks) != 2000 {
				t.Errorf("reader saw %d entries, want 2000: the index was published half written", len(idx.Frameworks))
				return
			}
		}
	}()

	wg.Wait()
}

func TestWriteCachedIndex_leavesNoTempFileBehind(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	for i := 0; i < 3; i++ {
		writeCachedIndex(indexBytes("round"+strconv.Itoa(i), 10))
	}

	entries, err := os.ReadDir(filepath.Dir(config.StoreIndexFile()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file survived the write: %s", e.Name())
		}
	}
}
