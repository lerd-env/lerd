package ui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// changeFeed nudges every subscriber when something changes. A nudge carries no
// data and collapses with any still unread, so a slow page never stalls it.
type changeFeed struct {
	mu     sync.Mutex
	subs   map[chan struct{}]struct{}
	cancel context.CancelFunc
	start  func(ctx context.Context, onChange func()) error
}

func newChangeFeed(start func(ctx context.Context, onChange func()) error) *changeFeed {
	return &changeFeed{subs: map[chan struct{}]struct{}{}, start: start}
}

// subscribe starts the feed's source when nothing runs it yet, which is also
// how a start that failed gets retried by the next page.
func (f *changeFeed) subscribe() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		if err := f.start(ctx, f.notify); err != nil {
			cancel()
		} else {
			f.cancel = cancel
		}
	}
	ch := make(chan struct{}, 1)
	f.subs[ch] = struct{}{}
	return ch
}

// unsubscribe stops the source with the last page, so lerd-ui watches nothing
// while no request list is open.
func (f *changeFeed) unsubscribe(ch chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.subs, ch)
	if len(f.subs) == 0 && f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
}

func (f *changeFeed) subscribers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

func (f *changeFeed) notify() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// requestsFeed fires when the watcher writes requests to the durable store. One
// inotify watch on the database and its WAL serves every dashboard socket.
var requestsFeed = newChangeFeed(func(ctx context.Context, onChange func()) error {
	db := config.RequestStatsDB()
	names := []string{filepath.Base(db), filepath.Base(db) + "-wal"}
	return watchDir(ctx, filepath.Dir(db), names, 100*time.Millisecond, onChange)
})

// requestsWatch is one dashboard socket's interest in a site's recent requests.
// The page names the site it shows and gets a nudge to reload whenever a newer
// request than the last one it saw lands for it.
type requestsWatch struct {
	domain, branch, key string
	last                reqstats.Record
	ch                  chan struct{}
}

// watch moves the interest to a site, or drops it for an empty domain, which is
// how a page that left the site says so. It returns a first nudge, since the
// page loaded its list before the watch began and a request may sit between.
func (w *requestsWatch) watch(domain, branch string) []byte {
	w.stop()
	if domain == "" {
		return nil
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		return nil
	}
	w.domain, w.branch, w.key = domain, branch, reqstats.Key(site.Name, branch)
	w.last = latestRequest(w.key)
	w.ch = requestsFeed.subscribe()
	return w.frame()
}

func (w *requestsWatch) stop() {
	if w.ch != nil {
		requestsFeed.unsubscribe(w.ch)
	}
	*w = requestsWatch{}
}

// events is nil while nothing is watched, which a select never picks.
func (w *requestsWatch) events() <-chan struct{} { return w.ch }

// changed reports the frame to send when the site has a request newer than the
// last one the page was told about. The store changes for every site, so most
// nudges end here with nothing to send.
func (w *requestsWatch) changed() ([]byte, bool) {
	r := latestRequest(w.key)
	if !newerRequest(r, w.last) {
		return nil, false
	}
	w.last = r
	return w.frame(), true
}

func (w *requestsWatch) frame() []byte {
	b, _ := json.Marshal(map[string]string{"type": "requests", "domain": w.domain, "branch": w.branch})
	return b
}

// newerRequest orders by time, then by row, because two requests can share a
// millisecond and a rowid can come back once the newest row was removed.
func newerRequest(r, than reqstats.Record) bool {
	if !r.At.Equal(than.At) {
		return r.At.After(than.At)
	}
	return r.ID > than.ID
}

// latestRequest is the newest listed request for a stats key, read through
// Recent so a static asset or an excluded route never nudges.
func latestRequest(key string) reqstats.Record {
	store, err := getAnalyticsStore()
	if err != nil {
		return reqstats.Record{}
	}
	recent, err := store.Recent(key, 1)
	if err != nil || len(recent) == 0 {
		return reqstats.Record{}
	}
	return recent[0]
}
