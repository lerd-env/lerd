package ui

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/push"
)

type dumpsSubscriber interface {
	Subscribe() (<-chan dumps.Event, func())
}

const dumpPreviewMax = 140

// dumpPreview collapses whitespace and trims a dump's Text into a single
// readable line that fits inside an OS notification body.
func dumpPreview(text string) string {
	if text == "" {
		return ""
	}
	flat := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, text)
	flat = strings.Join(strings.Fields(flat), " ")
	if len(flat) > dumpPreviewMax {
		cut := dumpPreviewMax - 1
		for cut > 0 && !utf8.RuneStart(flat[cut]) {
			cut--
		}
		flat = flat[:cut] + "…"
	}
	return flat
}

const dumpDebounceWindow = 5 * time.Second

// dumpDebouncer suppresses dump-notification floods. Two ray() calls from
// the same site within window collapse into a single notification.
type dumpDebouncer struct {
	mu     sync.Mutex
	window time.Duration
	last   map[string]time.Time
}

func newDumpDebouncer(window time.Duration) *dumpDebouncer {
	return &dumpDebouncer{window: window, last: map[string]time.Time{}}
}

func (d *dumpDebouncer) allow(site string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if t, ok := d.last[site]; ok && now.Sub(t) < d.window {
		return false
	}
	d.last[site] = now
	return true
}

func notificationForDump(evt dumps.Event) push.Notification {
	site := evt.Ctx.Site
	if site == "" {
		site = "(unknown site)"
	}
	kind := evt.Ctx.Type
	if kind == "" {
		kind = "dump"
	}
	preview := dumpPreview(evt.Text)
	body := preview
	if body == "" {
		body = kind + " dump captured (no text)"
	}
	return push.Notification{
		Kind:     "dump",
		TitleKey: "notify_dump_title",
		Title:    "Dump from " + site,
		BodyKey:  "notify_dump_body",
		Body:     body,
		Params: map[string]string{
			"site": site,
			"kind": kind,
			"text": body,
		},
		Tag:     "lerd-dump-" + site,
		URL:     debugRouteForContext(evt.Ctx),
		Data:    map[string]string{"site": site, "id": evt.ID},
		Urgency: "low",
		TTL:     60,
	}
}

// notificationForFailedJob reports one queued job that ended in a failed state.
// A worker draining a queue is otherwise silent, so this is the one job event
// worth interrupting for.
func notificationForFailedJob(evt dumps.Event) push.Notification {
	site := evt.Ctx.Site
	if site == "" {
		site = "(unknown site)"
	}
	var d struct {
		Class     string `json:"class"`
		Status    string `json:"status"`
		Queue     string `json:"queue"`
		Exception string `json:"exception"`
	}
	_ = json.Unmarshal(evt.Data, &d)
	job := d.Class
	if job == "" {
		job = "a queued job"
	}
	reason := dumpPreview(d.Exception)
	if reason == "" {
		reason = "no exception message was captured"
	}
	return push.Notification{
		Kind:     "job_failed",
		TitleKey: "notify_job_failed_title",
		Title:    "Job failed on " + site,
		BodyKey:  "notify_job_failed_body",
		Body:     job + ": " + reason,
		Params: map[string]string{
			"site":  site,
			"job":   job,
			"error": reason,
		},
		Tag:     "lerd-job-failed-" + site + "-" + job,
		URL:     debugRouteForContext(evt.Ctx),
		Data:    map[string]string{"site": site, "id": evt.ID},
		Urgency: "high",
		TTL:     300,
	}
}

// failedJobStatus is the job status that warrants a notification. Every other
// state a job passes through is progress, which the Debug window already shows.
const failedJobStatus = "failed"

// jobStatus reads the status off a job event without decoding the rest.
func jobStatus(evt dumps.Event) string {
	var d struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(evt.Data, &d)
	return d.Status
}

// notifyDispatch is the dispatch seam runDumpsNotifier uses, swappable in
// tests to capture what would have been notified.
var notifyDispatch = dispatchNotification

// runDumpsNotifier subscribes to the dumps server and dispatches one
// debounced notification per site per window for incoming dump events.
// Exits when the source closes the subscriber channel.
func runDumpsNotifier(src dumpsSubscriber) {
	if src == nil {
		return
	}
	ch, _ := src.Subscribe()
	d := newDumpDebouncer(dumpDebounceWindow)
	// Jobs debounce on their own clock: a job that fails is usually retried, so
	// the same class failing three times in a row is one thing to be told about.
	jobs := newDumpDebouncer(dumpDebounceWindow)
	np := newNPlusOneTracker()
	for evt := range ch {
		switch evt.Kind {
		case dumps.KindDump:
			if d.allow(evt.Ctx.Site) {
				notifyDispatch(notificationForDump(evt))
			}
		case dumps.KindQuery:
			// Queries are far too high-volume to notify on individually, but a
			// repeated query shape within one request is an N+1 worth a single
			// warning per route/script.
			if n := np.observe(evt); n != nil {
				notifyDispatch(*n)
			}
		case dumps.KindJob:
			if jobStatus(evt) != failedJobStatus {
				continue
			}
			n := notificationForFailedJob(evt)
			if jobs.allow(n.Tag) {
				notifyDispatch(n)
			}
		}
	}
}
