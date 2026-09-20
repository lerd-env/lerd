package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/push"
)

// fakeSubscriber feeds a fixed set of events to runDumpsNotifier then closes.
type fakeSubscriber struct{ evs []dumps.Event }

func (f *fakeSubscriber) Subscribe() (<-chan dumps.Event, func()) {
	ch := make(chan dumps.Event, len(f.evs))
	for _, e := range f.evs {
		ch <- e
	}
	close(ch)
	return ch, func() {}
}

func TestRunDumpsNotifier_OnlyNotifiesDumpKind(t *testing.T) {
	var got []dumps.Event
	prev := notifyDispatch
	notifyDispatch = func(n push.Notification) { got = append(got, dumps.Event{ID: n.Data["id"]}) }
	t.Cleanup(func() { notifyDispatch = prev })

	src := &fakeSubscriber{evs: []dumps.Event{
		{ID: "q1", Kind: dumps.KindQuery, Ctx: dumps.Context{Site: "a"}},
		{ID: "d1", Kind: dumps.KindDump, Ctx: dumps.Context{Site: "b"}},
		{ID: "j1", Kind: dumps.KindJob, Ctx: dumps.Context{Site: "c"}, Data: json.RawMessage(`{"status":"processed"}`)},
	}}
	runDumpsNotifier(src)

	if len(got) != 1 || got[0].ID != "d1" {
		ids := make([]string, len(got))
		for i, e := range got {
			ids[i] = e.ID
		}
		t.Errorf("expected only the dump to notify, got %v", ids)
	}
}

func TestNotificationForDump_Shape(t *testing.T) {
	evt := dumps.Event{ID: "abc", Kind: "dump", Ctx: dumps.Context{Site: "starlane.test", Type: "fpm"}}
	n := notificationForDump(evt)
	if n.Kind != "dump" {
		t.Errorf("Kind = %q", n.Kind)
	}
	if n.Params["site"] != "starlane.test" {
		t.Errorf("Params.site = %q", n.Params["site"])
	}
	if n.Params["kind"] != "fpm" {
		t.Errorf("Params.kind = %q", n.Params["kind"])
	}
	// No site is registered with that name in this test, so siteDomainForRoute
	// falls back to the input verbatim. The URL still lands on a sites sub-tab
	// route shape the frontend can parse.
	if n.URL != "#sites/starlane.test/dumps" {
		t.Errorf("URL = %q", n.URL)
	}
}

func TestNotificationForDump_BodyContainsDumpText(t *testing.T) {
	evt := dumps.Event{
		ID:   "abc",
		Kind: "dump",
		Ctx:  dumps.Context{Site: "starlane.test", Type: "fpm"},
		Text: "string(5) \"hello\"",
	}
	n := notificationForDump(evt)
	if n.Body != "string(5) \"hello\"" {
		t.Errorf("Body = %q, want dump text passed through", n.Body)
	}
	if n.Params["text"] != "string(5) \"hello\"" {
		t.Errorf("Params.text = %q", n.Params["text"])
	}
}

func TestNotificationForDump_TextTruncatedAndSingleLine(t *testing.T) {
	long := "line1\nline2  with   extra spaces\n" + string(make([]byte, 300))
	evt := dumps.Event{
		ID:   "abc",
		Kind: "dump",
		Ctx:  dumps.Context{Site: "x", Type: "fpm"},
		Text: long,
	}
	n := notificationForDump(evt)
	if len(n.Body) > 160 {
		t.Errorf("Body too long: %d chars", len(n.Body))
	}
	for _, c := range n.Body {
		if c == '\n' || c == '\r' {
			t.Errorf("Body contains newlines: %q", n.Body)
			break
		}
	}
}

// Truncating a multi-byte rune mid-byte produced � replacement chars
// in the notification body. Build a text whose 139-byte cut would split a
// rune and assert no replacement chars sneak in.
func TestDumpPreview_UTF8BoundarySafe(t *testing.T) {
	// 47 × 3-byte rune = 141 bytes — first 139 bytes lands mid-rune.
	text := strings.Repeat("☃", 47)
	got := dumpPreview(text)
	if strings.ContainsRune(got, '�') {
		t.Errorf("preview contains U+FFFD replacement char: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("preview should end with ellipsis: %q", got)
	}
}

func TestNotificationForDump_EmptyTextFallsBack(t *testing.T) {
	evt := dumps.Event{ID: "abc", Kind: "dump", Ctx: dumps.Context{Site: "x", Type: "fpm"}}
	n := notificationForDump(evt)
	if n.Body == "" {
		t.Error("Body should fall back to a description, not be empty")
	}
}

func TestDumpDebouncer_FirstEventPasses(t *testing.T) {
	d := newDumpDebouncer(time.Second)
	if !d.allow("a.test") {
		t.Error("first event for site should pass")
	}
}

func TestDumpDebouncer_SecondEventWithinWindowBlocked(t *testing.T) {
	d := newDumpDebouncer(time.Second)
	d.allow("a.test")
	if d.allow("a.test") {
		t.Error("second event within debounce window should be blocked")
	}
}

func TestDumpDebouncer_SecondEventAfterWindowPasses(t *testing.T) {
	d := newDumpDebouncer(10 * time.Millisecond)
	d.allow("a.test")
	time.Sleep(20 * time.Millisecond)
	if !d.allow("a.test") {
		t.Error("event after window should pass")
	}
}

func TestDumpDebouncer_DifferentSitesIndependent(t *testing.T) {
	d := newDumpDebouncer(time.Hour)
	if !d.allow("a.test") {
		t.Error("a.test first should pass")
	}
	if !d.allow("b.test") {
		t.Error("b.test should pass independently of a.test")
	}
}

// The dump-bridge tags Ctx.Site with the registered site name (the value
// of LERD_SITE), but the dashboard router keys the Sites tab by primary
// domain. notificationForDump must resolve name → primary domain so the
// click handler lands on the right site detail.
func TestNotificationForDump_URLResolvesNameToDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name:    "rapids",
		Domains: []string{"harborlist.test"},
		Path:    t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}

	evt := dumps.Event{ID: "z", Kind: "dump", Ctx: dumps.Context{Site: "rapids", Type: "fpm"}}
	n := notificationForDump(evt)
	if n.URL != "#sites/harborlist.test/dumps" {
		t.Errorf("URL = %q, want #sites/harborlist.test/dumps", n.URL)
	}
}

// TestRunDumpsNotifier_NotifiesFailedJobs checks a job that ends in a failed
// state interrupts, while the states it passes through on the way do not: a
// draining queue would otherwise report nothing at all.
func TestRunDumpsNotifier_NotifiesFailedJobs(t *testing.T) {
	var got []push.Notification
	prev := notifyDispatch
	notifyDispatch = func(n push.Notification) { got = append(got, n) }
	t.Cleanup(func() { notifyDispatch = prev })

	src := &fakeSubscriber{evs: []dumps.Event{
		{ID: "j1", Kind: dumps.KindJob, Ctx: dumps.Context{Site: "acme"}, Data: json.RawMessage(`{"class":"App\\Jobs\\SendInvoice","status":"queued"}`)},
		{ID: "j2", Kind: dumps.KindJob, Ctx: dumps.Context{Site: "acme"}, Data: json.RawMessage(`{"class":"App\\Jobs\\SendInvoice","status":"processing"}`)},
		{ID: "j3", Kind: dumps.KindJob, Ctx: dumps.Context{Site: "acme"}, Data: json.RawMessage(`{"class":"App\\Jobs\\SendInvoice","status":"failed","exception":"mailer refused the message"}`)},
		// The retry of the same job is the same failure, not a second one.
		{ID: "j4", Kind: dumps.KindJob, Ctx: dumps.Context{Site: "acme"}, Data: json.RawMessage(`{"class":"App\\Jobs\\SendInvoice","status":"failed","exception":"mailer refused the message"}`)},
	}}
	runDumpsNotifier(src)

	if len(got) != 1 {
		t.Fatalf("got %d notifications, want one for the failed job: %+v", len(got), got)
	}
	n := got[0]
	if n.Kind != "job_failed" {
		t.Errorf("Kind = %q, want job_failed", n.Kind)
	}
	if n.Params["job"] != `App\Jobs\SendInvoice` {
		t.Errorf("Params.job = %q", n.Params["job"])
	}
	if !strings.Contains(n.Body, "mailer refused the message") {
		t.Errorf("Body = %q, want the exception in it", n.Body)
	}
	if n.Data["id"] != "j3" {
		t.Errorf("Data.id = %q, want the failed event", n.Data["id"])
	}
}

// TestNotificationForFailedJob_FallsBackWithoutDetail checks a job event with
// nothing but a status still reads as something rather than an empty body.
func TestNotificationForFailedJob_FallsBackWithoutDetail(t *testing.T) {
	n := notificationForFailedJob(dumps.Event{ID: "x", Kind: dumps.KindJob, Data: json.RawMessage(`{"status":"failed"}`)})
	if n.Params["site"] != "(unknown site)" {
		t.Errorf("Params.site = %q", n.Params["site"])
	}
	if n.Params["job"] != "a queued job" || n.Params["error"] == "" {
		t.Errorf("job/error = %q/%q, want readable fallbacks", n.Params["job"], n.Params["error"])
	}
}

// TestNotificationForMessage_NamesWhereItWentAndWhatItSaid covers the row a
// developer is told about: nothing catches an SMS, so the notification is the
// only sign it left the machine.
func TestNotificationForMessage_NamesWhereItWentAndWhatItSaid(t *testing.T) {
	evt := dumps.Event{
		ID:   "m1",
		Kind: dumps.KindMessage,
		Ctx:  dumps.Context{Site: "acme", Type: "fpm"},
		Data: json.RawMessage(`{"channel":"sms","transport":"twilio","to":"+40711000000","body":"your order shipped"}`),
	}
	n := notificationForMessage(evt)

	if n.Kind != "message" || n.Data["id"] != "m1" {
		t.Errorf("notification = %+v, want the message kind and the event id", n)
	}
	if !strings.Contains(n.Title, "acme") {
		t.Errorf("title = %q, want the site that sent it", n.Title)
	}
	for _, want := range []string{"sms", "twilio", "+40711000000", "your order shipped"} {
		if !strings.Contains(n.Body, want) {
			t.Errorf("body = %q, want it to carry %q", n.Body, want)
		}
	}
}

// TestNotificationForMessage_FallsBackToWhatBuiltIt covers a Laravel
// notification, whose text lives in code lerd must not run: the class that
// built it is what names the message instead.
func TestNotificationForMessage_FallsBackToWhatBuiltIt(t *testing.T) {
	evt := dumps.Event{
		ID:   "m2",
		Kind: dumps.KindMessage,
		Ctx:  dumps.Context{Site: "acme"},
		Data: json.RawMessage(`{"channel":"vonage","to":"+40711000000","notification":"App\\Notifications\\OrderShipped"}`),
	}
	n := notificationForMessage(evt)

	if !strings.Contains(n.Body, "OrderShipped") || !strings.Contains(n.Body, "+40711000000") {
		t.Errorf("body = %q, want the notification class and the recipient", n.Body)
	}
}

// TestRunDumpsNotifier_NotifiesOneMessagePerWindow keeps a notification fanned
// out to several recipients from arriving as several pop-ups.
func TestRunDumpsNotifier_NotifiesOneMessagePerWindow(t *testing.T) {
	var got []string
	prev := notifyDispatch
	notifyDispatch = func(n push.Notification) { got = append(got, n.Kind+":"+n.Data["id"]) }
	t.Cleanup(func() { notifyDispatch = prev })

	msg := func(id string) dumps.Event {
		return dumps.Event{ID: id, Kind: dumps.KindMessage, Ctx: dumps.Context{Site: "acme"}, Data: json.RawMessage(`{"channel":"sms","body":"hi"}`)}
	}
	runDumpsNotifier(&fakeSubscriber{evs: []dumps.Event{msg("m1"), msg("m2"), msg("m3")}})

	if len(got) != 1 || got[0] != "message:m1" {
		t.Errorf("notified %v, want one message per site per window", got)
	}
}

// TestNotificationForMessage_TagsEachSendOnItsOwn keeps a second message from
// replacing the first in the tray: a repeated tag updates a notification in
// place, which reads as nothing happening.
func TestNotificationForMessage_TagsEachSendOnItsOwn(t *testing.T) {
	data := json.RawMessage(`{"channel":"sms","body":"hi"}`)
	a := notificationForMessage(dumps.Event{ID: "m1", Kind: dumps.KindMessage, Ctx: dumps.Context{Site: "acme"}, Data: data})
	b := notificationForMessage(dumps.Event{ID: "m2", Kind: dumps.KindMessage, Ctx: dumps.Context{Site: "acme"}, Data: data})

	if a.Tag == b.Tag {
		t.Errorf("both sends tagged %q, want one tag per send", a.Tag)
	}
	if !strings.Contains(a.Tag, "acme") {
		t.Errorf("tag = %q, want the site in it", a.Tag)
	}
}
