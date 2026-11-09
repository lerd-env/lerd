package plugins

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFailed_RecordsAndNotifiesOnlyWhatIsNew(t *testing.T) {
	isolate(t)
	var told []Failure
	orig := notifyFailure
	notifyFailure = func(f Failure) { told = append(told, f) }
	t.Cleanup(func() { notifyFailure = orig })

	f := Failure{Plugin: "hello", Surface: SurfaceHook, What: "site.linked", Error: "boom"}
	Failed(f)
	Failed(f)
	if len(told) != 1 {
		t.Fatalf("the same failure again is not news, notified %d times", len(told))
	}
	Failed(Failure{Plugin: "hello", Surface: SurfaceHook, What: "site.linked", Error: "another"})
	if len(told) != 2 {
		t.Fatalf("a different failure is news, notified %d times", len(told))
	}
	last, ok := LastFailure("hello")
	if !ok || last.Error != "another" || last.At.IsZero() {
		t.Errorf("last failure = %+v %v", last, ok)
	}
}

func TestFailed_SameFailureIsNewsAgainAfterAnHour(t *testing.T) {
	isolate(t)
	n := 0
	orig := notifyFailure
	notifyFailure = func(Failure) { n++ }
	t.Cleanup(func() { notifyFailure = orig })
	f := Failure{Plugin: "hello", Surface: SurfaceAPI, What: "greet", Error: "boom"}
	Failed(f)
	old, _ := LastFailure("hello")
	old.At = time.Now().Add(-2 * time.Hour)
	writeFailure(old)
	Failed(f)
	if n != 2 {
		t.Errorf("notified %d times, want 2", n)
	}
}

func TestReportURL(t *testing.T) {
	isolate(t)
	f := Failure{Plugin: "hello", Surface: SurfaceHook, What: "site.linked", Error: "line one\nline two"}
	if got := f.ReportURL(); got != "" {
		t.Errorf("a plugin with no known repository has nowhere to report, got %q", got)
	}
	writePlugin(t, Dir(), "hello", "api_version: 1\nname: hello\nversion: 1.2.0\n")
	if err := writeSource(Dir()+"/hello", &Source{Repo: "https://github.com/acme/hello", Commit: strings.Repeat("c", 40)}); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(f.ReportURL())
	if err != nil || u.Host != "github.com" || u.Path != "/acme/hello/issues/new" {
		t.Fatalf("url %v %v", u, err)
	}
	title, body := u.Query().Get("title"), u.Query().Get("body")
	if !strings.Contains(title, "site.linked") || !strings.Contains(title, "line one") || strings.Contains(title, "line two") {
		t.Errorf("title %q", title)
	}
	for _, want := range []string{strings.Repeat("c", 40), "hook", "line one\nline two", runningVersion, "github.com/lerd-env/lerd/issues"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
}

func TestReportURL_KeepsTheLinkShort(t *testing.T) {
	isolate(t)
	writePlugin(t, Dir(), "hello", "api_version: 1\nname: hello\n")
	if err := writeSource(Dir()+"/hello", &Source{Repo: "https://github.com/acme/hello", Commit: strings.Repeat("c", 40)}); err != nil {
		t.Fatal(err)
	}
	f := Failure{Plugin: "hello", Surface: SurfaceAPI, What: "greet", Error: strings.Repeat("x", 20000)}
	if got := f.ReportURL(); len(got) > 8000 {
		t.Errorf("a report link of %d bytes is refused by GitHub", len(got))
	}
}

func TestFailures_KeepsARecentHistoryNewestFirst(t *testing.T) {
	isolate(t)
	for i := 0; i < failureHistory+5; i++ {
		Record(Failure{Plugin: "hello", Surface: SurfaceCLI, What: "greet", Error: "boom " + string(rune('a'+i%26))})
	}
	Record(Failure{Plugin: "hello", Surface: SurfaceHook, What: "site.linked", Error: "latest"})
	got := Failures("hello")
	if len(got) != failureHistory {
		t.Fatalf("kept %d failures, want the last %d", len(got), failureHistory)
	}
	if got[0].Error != "latest" || got[0].At.IsZero() {
		t.Errorf("newest first, got %+v", got[0])
	}
	if len(Failures("other")) != 0 {
		t.Error("another plugin has no history")
	}
	ClearFailure("hello")
	if len(Failures("hello")) != 0 {
		t.Error("ClearFailure must forget the history too")
	}
}

func TestFailures_FallsBackToTheLatestKeptBeforeTheLog(t *testing.T) {
	isolate(t)
	writeFailure(Failure{Plugin: "hello", Surface: SurfaceHook, What: "site.linked", Error: "old"})
	if got := Failures("hello"); len(got) != 1 || got[0].Error != "old" {
		t.Errorf("Failures = %+v, want the latest failure alone", got)
	}
}

// Failures recorded at the same moment, by the dashboard and a CLI command
// say, must all reach the log rather than overwrite each other.
func TestFailures_KeepsEveryOneRecordedAtOnce(t *testing.T) {
	isolate(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Record(Failure{Plugin: "hello", Surface: SurfaceAPI, What: "greet", Error: fmt.Sprintf("boom %d", i)})
		}(i)
	}
	wg.Wait()
	if got := len(Failures("hello")); got != 30 {
		t.Errorf("kept %d of 30 failures", got)
	}
}

// A command that runs for days writes far more than a failure keeps, so the
// tail holds on to the end and nothing else.
func TestTail_KeepsOnlyWhatAFailureKeeps(t *testing.T) {
	var tail Tail
	line := strings.Repeat("x", 999) + "\n"
	for i := 0; i < 100; i++ {
		_, _ = tail.Write([]byte(line))
	}
	_, _ = tail.Write([]byte(strings.Repeat("y", 3*failureKeep)))
	_, _ = tail.Write([]byte("error: the end\n"))

	got := tail.String()
	if len(got) != failureKeep {
		t.Fatalf("tail holds %d bytes, want %d", len(got), failureKeep)
	}
	if !strings.HasSuffix(got, "yyyerror: the end\n") {
		t.Errorf("tail lost the end of the output: %q", got[len(got)-40:])
	}
	if cap(tail.buf) > 2*failureKeep {
		t.Errorf("tail grew to %d bytes of capacity", cap(tail.buf))
	}
}

func TestTail_KeepsShortOutputWhole(t *testing.T) {
	var tail Tail
	_, _ = tail.Write([]byte("cargo: "))
	_, _ = tail.Write([]byte("not found\n"))
	if got := tail.String(); got != "cargo: not found\n" {
		t.Errorf("tail = %q", got)
	}
}

func TestTail_StartsAtAWholeCharacter(t *testing.T) {
	var tail Tail
	_, _ = tail.Write([]byte(strings.Repeat("é", failureKeep)))
	got := tail.String()
	if !utf8.ValidString(got) || !strings.HasPrefix(got, "é") {
		t.Errorf("tail starts mid-character: %q", got[:4])
	}
}
