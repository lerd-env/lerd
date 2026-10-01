package watcher

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// frameworkSignals is the set of files whose creation in a project directory
// signals that a supported PHP framework has been set up there.
var frameworkSignals = map[string]bool{
	"artisan":       true, // Laravel
	"composer.json": true, // Symfony, Drupal, Bedrock WordPress, etc.
	"wp-login.php":  true, // Traditional WordPress (no Composer)
}

// Watch monitors the given directories for new and deleted project subdirectories.
// onNew is called when a framework signal file appears in a direct subdirectory of a parked dir.
// onRemoved is called when a watched subdirectory is deleted.
// It returns when ctx is canceled, so a shutdown unblocks it instead of killing the process.
func Watch(ctx context.Context, dirs func() []string, onNew func(path string), onRemoved func(path string)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	// parkedDirs tracks the top-level parked directories so we only register
	// projects that are direct children of them, not deeper nestings.
	parkedDirs := map[string]bool{}
	reconcileParked(w, parkedDirs, dirs())

	// lerd park and unpark only edit the config, so the list is read again
	// rather than taken once at start.
	tick := time.NewTicker(parkedReconcileEvery)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			reconcileParked(w, parkedDirs, dirs())
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			switch {
			case event.Op&fsnotify.Remove != 0:
				onRemoved(event.Name)
			case event.Op&(fsnotify.Create|fsnotify.Write) != 0:
				if frameworkSignals[filepath.Base(event.Name)] {
					projectDir := filepath.Dir(event.Name)
					// Only register if this is a direct child of a parked dir.
					if parkedDirs[filepath.Dir(projectDir)] {
						onNew(projectDir)
					}
				} else if event.Op&fsnotify.Create != 0 {
					// New direct subdirectory in a parked dir — watch it for framework signal files.
					if parkedDirs[filepath.Dir(event.Name)] {
						if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
							if err := w.Add(event.Name); err != nil {
								logger.Error("failed to watch new subdirectory", "path", event.Name, "err", err)
							} else {
								logger.Debug("watching new subdirectory", "path", event.Name)
								// Files written before the watch was added sent
								// their events to nobody, so look for them now.
								if hasFrameworkSignal(event.Name) {
									onNew(event.Name)
								}
							}
						}
					}
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			logger.Error("fsnotify error", "err", err)
		}
	}
}

func hasFrameworkSignal(dir string) bool {
	for name := range frameworkSignals {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func expandHome(path string) string {
	if len(path) > 1 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// parkedReconcileEvery is how often Watch reads the parked list again.
var parkedReconcileEvery = 5 * time.Second

// reconcileParked watches every directory in dirs that is not watched yet,
// with its direct subdirectories, and stops watching the ones no longer parked.
func reconcileParked(w *fsnotify.Watcher, parkedDirs map[string]bool, dirs []string) {
	want := map[string]bool{}
	for _, dir := range dirs {
		want[expandHome(dir)] = true
	}
	for dir := range parkedDirs {
		if !want[dir] {
			_ = w.Remove(dir)
			delete(parkedDirs, dir)
		}
	}
	for expanded := range want {
		if parkedDirs[expanded] {
			continue
		}
		if err := os.MkdirAll(expanded, 0755); err != nil {
			continue
		}
		if err := w.Add(expanded); err != nil {
			continue
		}
		parkedDirs[expanded] = true
		// Also watch existing direct subdirectories so we catch framework signal files inside them.
		entries, _ := os.ReadDir(expanded)
		for _, e := range entries {
			if e.IsDir() {
				sub := filepath.Join(expanded, e.Name())
				if err := w.Add(sub); err != nil {
					logger.Error("failed to watch subdirectory", "path", sub, "err", err)
				}
			}
		}
	}
}
