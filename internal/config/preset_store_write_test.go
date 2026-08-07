package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// storePresetYAML builds a preset long enough that a truncated write lands
// mid-file. The versions list is last, so a cut above it still parses.
func storePresetYAML(tag string) []byte {
	var sb strings.Builder
	sb.WriteString("name: redis\n")
	sb.WriteString("description: " + strings.Repeat("padding ", 200) + "\n")
	sb.WriteString("versions:\n")
	for i := 0; i < 40; i++ {
		v := tag + "." + strconv.Itoa(i)
		sb.WriteString("    - tag: \"" + v + "\"\n")
		sb.WriteString("      image: example/redis:" + v + "\n")
	}
	return []byte(sb.String())
}

// The service store cache has the same shape of writer as the framework store:
// the fetch hook refreshes a preset past its staleness window while other
// processes are reading it. A preset cut above its versions key still parses.
func TestSaveStorePreset_readerNeverSeesAPartialPreset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := filepath.Join(StorePresetsDir(), "redis.yaml")

	if err := SaveStorePreset("redis", storePresetYAML("1")); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if err := SaveStorePreset("redis", storePresetYAML(strconv.Itoa(i))); err != nil {
				t.Errorf("save: %v", err)
				break
			}
		}
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
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if err := ValidatePresetYAML(data, "redis"); err != nil {
				t.Errorf("reader saw a half written preset: %v", err)
				return
			}
			if n := strings.Count(string(data), "    - tag:"); n != 40 {
				t.Errorf("reader saw %d versions, want 40: the preset was published half written", n)
				return
			}
		}
	}()

	wg.Wait()
}

// EnsurePreset refreshes on a staleness tick, and the bytes it fetches are
// nearly always the ones already on disk.
func TestSaveStorePreset_skipsTheWriteWhenNothingChanged(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := filepath.Join(StorePresetsDir(), "redis.yaml")

	if err := SaveStorePreset("redis", storePresetYAML("1")); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := SaveStorePreset("redis", storePresetYAML("1")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged preset was republished")
	}
}

func TestSaveStorePreset_leavesNoTempFileBehind(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := StorePresetsDir()

	for i := 0; i < 3; i++ {
		if err := SaveStorePreset("redis", storePresetYAML(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "redis.yaml" {
			t.Errorf("unexpected leftover in the preset cache dir: %s", e.Name())
		}
	}
}
