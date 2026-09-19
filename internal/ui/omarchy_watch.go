package ui

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchOmarchyTheme calls onChange when the desktop theme is swapped. The watch
// sits on the state directory rather than on the theme itself because
// omarchy-theme-set stages the new theme beside the old one and moves it into
// place, so the path being watched would be the one that goes away.
//
// A missing directory is an error the caller is meant to ignore: most machines
// have no Omarchy, and that is not a failure of the daemon.
func watchOmarchyTheme(ctx context.Context, current string, settle time.Duration, onChange func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(current); err != nil {
		_ = w.Close()
		return err
	}
	go func() {
		defer func() { _ = w.Close() }()
		// One swap writes the directory and the name file, so events are
		// collapsed into a single report rather than repainting every open
		// dashboard once per file.
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
				switch filepath.Base(event.Name) {
				case "theme", "theme.name":
				default:
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
