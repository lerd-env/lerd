package watcher

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestWatchSourceFiles_savesFireActivity asserts a write under a watched source
// tree reports activity for the right key, while a write under an excluded
// subtree (node_modules) is ignored.
func TestWatchSourceFiles_savesFireActivity(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	nmDir := filepath.Join(root, "node_modules")
	for _, d := range []string{srcDir, nmDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	activity := make(chan string, 8)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		_ = WatchSourceFiles(
			func() []SourceTarget {
				return []SourceTarget{{Key: "mysite", Dirs: []string{root}}}
			},
			40*time.Millisecond,
			func(key string) { activity <- key },
			stop,
		)
	}()
	// Let the initial scan register the directory watches.
	time.Sleep(300 * time.Millisecond)

	// A save in the source tree reports activity for the target's key.
	if err := os.WriteFile(filepath.Join(srcDir, "Foo.vue"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-activity:
		if key != "mysite" {
			t.Errorf("activity key = %q, want mysite", key)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no activity reported for a source-file save")
	}

	// A write under node_modules is excluded and must not report activity.
	if err := os.WriteFile(filepath.Join(nmDir, "dep.js"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-activity:
		t.Errorf("node_modules write reported activity (%q); it must be excluded", key)
	case <-time.After(600 * time.Millisecond):
		// no activity — correct
	}
}

// TestWatchSourceFiles_skipsOversizedDir asserts a directory dense enough to be
// an asset dump (more than maxSourceDirEntries files, e.g. a checked-in icon
// set) is not watched, so kqueue's per-file fds can't exhaust the process limit.
func TestWatchSourceFiles_skipsOversizedDir(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	assets := filepath.Join(root, "src", "icons") // dense asset dump under a source root
	for _, d := range []string{srcDir, assets} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i <= maxSourceDirEntries; i++ { // > maxSourceDirEntries entries
		if err := os.WriteFile(filepath.Join(assets, "i"+strconv.Itoa(i)+".svg"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	activity := make(chan string, 8)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		_ = WatchSourceFiles(
			func() []SourceTarget { return []SourceTarget{{Key: "mysite", Dirs: []string{root}}} },
			40*time.Millisecond,
			func(key string) { activity <- key },
			stop,
		)
	}()
	time.Sleep(300 * time.Millisecond)

	// A write in the oversized dir must not report activity: it was skipped.
	if err := os.WriteFile(filepath.Join(assets, "new.svg"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-activity:
		t.Errorf("oversized-dir write reported activity (%q); it must be skipped", key)
	case <-time.After(600 * time.Millisecond):
		// no activity, correct
	}

	// The normal source root is still watched.
	if err := os.WriteFile(filepath.Join(srcDir, "App.vue"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-activity:
		if key != "mysite" {
			t.Errorf("activity key = %q, want mysite", key)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no activity for a normal source-file save (over-broad skip?)")
	}
}

// Re-walking a tree costs a readdir per directory, so an incremental rescan must
// only look at roots that aren't watched yet.
func TestRootsToWalk_incrementalSkipsWatchedRoots(t *testing.T) {
	targets := []SourceTarget{
		{Key: "a", Dirs: []string{"/p/a/src", "/p/a/app"}},
		{Key: "b", Dirs: []string{"/p/b/src"}},
	}
	walked := map[string]bool{"/p/a/src": true, "/p/a/app": true}

	got := rootsToWalk(targets, walked, false)
	if len(got) != 1 || got[0].Key != "b" {
		t.Fatalf("got %+v, want only the unwatched target b", got)
	}
	if len(got[0].Dirs) != 1 || got[0].Dirs[0] != "/p/b/src" {
		t.Errorf("dirs = %v, want only /p/b/src", got[0].Dirs)
	}
}

// A target only partly watched still has its remaining roots walked.
func TestRootsToWalk_incrementalKeepsUnwatchedDirsOfAWatchedTarget(t *testing.T) {
	targets := []SourceTarget{{Key: "a", Dirs: []string{"/p/a/src", "/p/a/app"}}}
	walked := map[string]bool{"/p/a/src": true}

	got := rootsToWalk(targets, walked, false)
	if len(got) != 1 || len(got[0].Dirs) != 1 || got[0].Dirs[0] != "/p/a/app" {
		t.Errorf("got %+v, want only /p/a/app", got)
	}
}

func TestRootsToWalk_incrementalWithNothingNewWalksNothing(t *testing.T) {
	targets := []SourceTarget{{Key: "a", Dirs: []string{"/p/a/src"}}}
	walked := map[string]bool{"/p/a/src": true}

	if got := rootsToWalk(targets, walked, false); len(got) != 0 {
		t.Errorf("got %+v, want nothing to walk", got)
	}
}

// The periodic resync exists to catch a directory created before its parent was
// watched, so it must return everything regardless of what's already walked.
func TestRootsToWalk_resyncReturnsEveryRoot(t *testing.T) {
	targets := []SourceTarget{
		{Key: "a", Dirs: []string{"/p/a/src"}},
		{Key: "b", Dirs: []string{"/p/b/src"}},
	}
	walked := map[string]bool{"/p/a/src": true, "/p/b/src": true}

	got := rootsToWalk(targets, walked, true)
	if len(got) != 2 {
		t.Errorf("resync returned %d targets, want all 2", len(got))
	}
}

// A site linked after the watcher started must still get watched, which is what
// the incremental pass is for.
func TestWatchSourceFiles_picksUpATargetAddedLater(t *testing.T) {
	oldInterval := sourceRescanInterval
	sourceRescanInterval = 60 * time.Millisecond
	t.Cleanup(func() { sourceRescanInterval = oldInterval })

	first := t.TempDir()
	later := t.TempDir()
	for _, d := range []string{filepath.Join(first, "src"), filepath.Join(later, "src")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	targets := []SourceTarget{{Key: "first", Dirs: []string{first}}}

	activity := make(chan string, 8)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		_ = WatchSourceFiles(
			func() []SourceTarget {
				mu.Lock()
				defer mu.Unlock()
				return append([]SourceTarget(nil), targets...)
			},
			30*time.Millisecond,
			func(key string) { activity <- key },
			stop,
		)
	}()
	time.Sleep(300 * time.Millisecond)

	// Register the second site the way linking one would.
	mu.Lock()
	targets = append(targets, SourceTarget{Key: "later", Dirs: []string{later}})
	mu.Unlock()
	time.Sleep(400 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(later, "src", "New.php"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-activity:
		if key != "later" {
			t.Errorf("activity key = %q, want later", key)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a target added after startup never got watched")
	}
}

// A directory created under an already-watched tree is picked up from the CREATE
// event, which is what lets the periodic full re-walk be rare.
func TestWatchSourceFiles_newSubdirWatchedWithoutAResync(t *testing.T) {
	oldInterval, oldEvery := sourceRescanInterval, sourceResyncEvery
	// Rescan often, but never resync, so only the CREATE path can succeed.
	sourceRescanInterval, sourceResyncEvery = 50*time.Millisecond, 1_000_000
	t.Cleanup(func() { sourceRescanInterval, sourceResyncEvery = oldInterval, oldEvery })

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	activity := make(chan string, 8)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		_ = WatchSourceFiles(
			func() []SourceTarget { return []SourceTarget{{Key: "mysite", Dirs: []string{root}}} },
			30*time.Millisecond,
			func(key string) { activity <- key },
			stop,
		)
	}()
	time.Sleep(300 * time.Millisecond)

	nested := filepath.Join(root, "src", "Http", "Controllers")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	// Drain activity caused by creating the directories themselves.
	for len(activity) > 0 {
		<-activity
	}

	if err := os.WriteFile(filepath.Join(nested, "Api.php"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-activity:
		// watched via the CREATE path — correct
	case <-time.After(3 * time.Second):
		t.Fatal("a subdirectory created under a watched tree never got watched")
	}
}
