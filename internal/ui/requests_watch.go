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
	mu    sync.Mutex
	subs  map[chan struct{}]struct{}
	once  sync.Once
	start func(onChange func()) error
}

func newChangeFeed(start func(onChange func()) error) *changeFeed {
	return &changeFeed{subs: map[chan struct{}]struct{}{}, start: start}
}

// subscribe starts the feed's source on first use, so lerd-ui watches nothing
// until a page asks.
func (f *changeFeed) subscribe() chan struct{} {
	f.once.Do(func() { _ = f.start(f.notify) })
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	f.subs[ch] = struct{}{}
	f.mu.Unlock()
	return ch
}

func (f *changeFeed) unsubscribe(ch chan struct{}) {
	f.mu.Lock()
	delete(f.subs, ch)
	f.mu.Unlock()
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
var requestsFeed = newChangeFeed(func(onChange func()) error {
	db := config.RequestStatsDB()
	names := []string{filepath.Base(db), filepath.Base(db) + "-wal"}
	return watchDir(context.Background(), filepath.Dir(db), names, 100*time.Millisecond, onChange)
})

// requestsWatch is one dashboard socket's interest in a site's recent requests.
// The page names the site it shows and gets a nudge to reload whenever a newer
// request than the last one it saw lands for it.
type requestsWatch struct {
	domain, branch, key string
	last                int64
	ch                  chan struct{}
}

// watch moves the interest to a site, or drops it for an empty domain, which is
// how a page that left the site says so.
func (w *requestsWatch) watch(domain, branch string) {
	w.stop()
	if domain == "" {
		return
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		return
	}
	w.domain, w.branch, w.key = domain, branch, reqstats.Key(site.Name, branch)
	w.last = latestRequestAt(w.key)
	w.ch = requestsFeed.subscribe()
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
	at := latestRequestAt(w.key)
	if at <= w.last {
		return nil, false
	}
	w.last = at
	b, _ := json.Marshal(map[string]string{"type": "requests", "domain": w.domain, "branch": w.branch})
	return b, true
}

// latestRequestAt is when the newest listed request for a stats key arrived,
// read through Recent so a static asset or an excluded route never nudges.
func latestRequestAt(key string) int64 {
	store, err := getAnalyticsStore()
	if err != nil {
		return 0
	}
	recent, err := store.Recent(key, 1)
	if err != nil || len(recent) == 0 {
		return 0
	}
	return recent[0].At.UnixMilli()
}
