package ui

import (
	"context"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDir calls onChange once a burst of writes to the named files in dir
// settles, such as a desktop repainting itself or the request store taking a
// batch. A missing directory is an error the theme caller ignores, since most
// machines have no desktop lerd can follow. Nil names reports any file.
func watchDir(ctx context.Context, dir string, names []string, settle time.Duration, onChange func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return err
	}
	go func() {
		defer func() { _ = w.Close() }()
		// One swap writes several of the watched names, so events are collapsed
		// into a single report rather than repainting every open dashboard once
		// per file.
		var timer *time.Timer
		var fire <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if names != nil && !slices.Contains(names, filepath.Base(event.Name)) {
					continue
				}
				if timer == nil {
					timer = time.NewTimer(settle)
					fire = timer.C
					continue
				}
				timer.Reset(settle)
			case <-fire:
				timer, fire = nil, nil
				onChange()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}
