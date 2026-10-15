package plugins

import (
	"net/url"
	"strings"
	"testing"
	"time"
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
