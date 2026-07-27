//go:build !nogui

package tray

import (
	"fmt"
	"sync"

	"github.com/getlantern/systray"
)

// row is one entry of a dynamic menu section: what to show, and which lerd
// command to run when it is clicked. A row without args is inert.
type row struct {
	title    string
	tooltip  string
	args     []string
	disabled bool
}

// fitRows clips rows to the pool size, folding the remainder into a trailing
// inert row so entries past the cap are reported instead of vanishing.
func fitRows(rows []row, size int) []row {
	if size <= 0 {
		return nil
	}
	if len(rows) <= size {
		return rows
	}
	out := make([]row, size)
	copy(out, rows[:size-1])
	out[size-1] = row{
		title:    fmt.Sprintf("… %d more", len(rows)-(size-1)),
		tooltip:  "Open the dashboard to see every entry",
		disabled: true,
	}
	return out
}

// menuList drives one dynamic section of the tray menu. systray cannot remove
// an item once added, so the pool is allocated up front and rebound on every
// poll; rows is the single source of truth for both rendering and clicks.
type menuList struct {
	items []*systray.MenuItem

	mu   sync.RWMutex
	rows []row
}

// newMenuList allocates a pool of size items nested under parent.
func newMenuList(parent *systray.MenuItem, size int) *menuList {
	l := &menuList{items: make([]*systray.MenuItem, size)}
	for i := range l.items {
		l.items[i] = parent.AddSubMenuItem("", "")
		l.items[i].Hide()
	}
	return l
}

// set rebinds the pool to rows and reports how many ended up visible.
func (l *menuList) set(rows []row) int {
	rows = fitRows(rows, len(l.items))

	l.mu.Lock()
	l.rows = rows
	l.mu.Unlock()

	for i, item := range l.items {
		if i >= len(rows) {
			item.Hide()
			continue
		}
		item.SetTitle(rows[i].title)
		item.SetTooltip(rows[i].tooltip)
		if rows[i].disabled {
			item.Disable()
		} else {
			item.Enable()
		}
		item.Show()
	}
	return len(rows)
}

// watch dispatches clicks for every slot, resolving the row at click time so a
// slot always acts on what it currently displays.
func (l *menuList) watch(refresh func()) {
	for i := range l.items {
		go func(idx int) {
			for range l.items[idx].ClickedCh {
				if args := l.argsAt(idx); len(args) > 0 {
					runAndRefresh(lerdCmd(args...), refresh)
				}
			}
		}(i)
	}
}

func (l *menuList) argsAt(idx int) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if idx >= len(l.rows) {
		return nil
	}
	return l.rows[idx].args
}
