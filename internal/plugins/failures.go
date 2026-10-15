package plugins

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Failure is the latest thing a plugin's executable got wrong, kept so the
// dashboard can show it and offer to report it to the plugin's author.
type Failure struct {
	Plugin  string    `json:"plugin"`
	Surface string    `json:"surface"`
	What    string    `json:"what"`
	Error   string    `json:"error"`
	At      time.Time `json:"at"`
}

// failureRepeat is how long the same failure stays old news.
const failureRepeat = time.Hour

// notifyFailure tells the user about a new failure. Each process sets it to
// reach the dashboard its own way; it does nothing until one does.
var notifyFailure = func(Failure) {}

// SetFailureNotifier sets how this process tells the user about a new failure.
func SetFailureNotifier(fn func(Failure)) { notifyFailure = fn }

func failuresDir() string { return filepath.Join(config.DataDir(), "plugin-failures") }

// Failed records f as the plugin's latest failure and tells the user when it
// is news: a different failure from the last, or the same one an hour later.
func Failed(f Failure) {
	if Record(f) {
		notifyFailure(f)
	}
}

// failureKeep bounds what is kept of a failure's output: its end, where the
// error usually is.
const failureKeep = 8000

// Record keeps f as the plugin's latest failure without telling anyone, for a
// surface where the user is already watching it fail, and reports whether it
// is news.
func Record(f Failure) bool {
	f.At = time.Now()
	if len(f.Error) > failureKeep {
		f.Error = "…" + f.Error[len(f.Error)-failureKeep:]
	}
	prev, ok := LastFailure(f.Plugin)
	writeFailure(f)
	return !ok || prev.Surface != f.Surface || prev.What != f.What || prev.Error != f.Error || f.At.Sub(prev.At) >= failureRepeat
}

// LastFailure returns the plugin's latest recorded failure.
func LastFailure(name string) (Failure, bool) {
	var f Failure
	data, err := os.ReadFile(filepath.Join(failuresDir(), name+".json"))
	if err != nil || json.Unmarshal(data, &f) != nil {
		return Failure{}, false
	}
	return f, true
}

// ClearFailure forgets the plugin's failure, once it is removed or replaced.
func ClearFailure(name string) {
	_ = os.Remove(filepath.Join(failuresDir(), name+".json"))
}

func writeFailure(f Failure) {
	if !nameRe.MatchString(f.Plugin) {
		return
	}
	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	path := filepath.Join(failuresDir(), f.Plugin+".json")
	config.GuardRealWrite(path)
	if os.MkdirAll(failuresDir(), 0o755) == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// reportErrorLimit keeps a report link well under what GitHub accepts.
const reportErrorLimit = 3000

// ReportURL opens a new issue on the plugin's repository, filled in with what
// its author needs. A plugin installed from a folder has nowhere to report to.
func (f Failure) ReportURL() string {
	src, ok := InstalledSource(f.Plugin)
	if !ok || !strings.HasPrefix(src.Repo, "https://github.com/") {
		return ""
	}
	errText := f.Error
	if len(errText) > reportErrorLimit {
		errText = errText[:reportErrorLimit] + "\n…"
	}
	first, _, _ := strings.Cut(errText, "\n")
	plugin := f.Plugin
	if p, err := Find(f.Plugin); err == nil && p.Version != "" {
		plugin += " " + p.Version
	}
	body := fmt.Sprintf("**Plugin:** %s at %s\n**Lerd:** %s on %s/%s\n**What failed:** %s %s\n\n```\n%s\n```\n\nIf this looks like a problem in Lerd itself rather than in the plugin, please pass it on to https://github.com/lerd-env/lerd/issues.\n",
		plugin, src.Commit, runningVersion, runtime.GOOS, runtime.GOARCH, f.Surface, f.What, errText)
	q := url.Values{}
	q.Set("title", fmt.Sprintf("%s %s failed: %s", f.Surface, f.What, first))
	q.Set("body", body)
	return strings.TrimSuffix(src.Repo, ".git") + "/issues/new?" + q.Encode()
}
