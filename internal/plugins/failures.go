package plugins

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

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

// Tail keeps the end of what is written to it, as much as a failure keeps, so
// collecting the output of a command that runs for days costs no more memory
// than collecting a short one.
type Tail struct{ buf []byte }

func (t *Tail) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= failureKeep {
		p = p[len(p)-failureKeep:]
		t.buf = t.buf[:0]
	}
	if over := len(t.buf) + len(p) - failureKeep; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	t.buf = append(t.buf, p...)
	return n, nil
}

// String starts at a whole character, since the cut can land inside one.
func (t *Tail) String() string {
	b := t.buf
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return string(b)
}

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
	appendHistory(f)
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

// ClearFailure forgets the plugin's failures, once it is removed or replaced.
func ClearFailure(name string) {
	_ = os.Remove(filepath.Join(failuresDir(), name+".json"))
	_ = os.Remove(historyPath(name))
	_ = os.Remove(historyPath(name) + ".lock")
}

// failureHistory is how many failures the dashboard's Logs tab can go back.
const failureHistory = 50

func historyPath(name string) string { return filepath.Join(failuresDir(), name+".log") }

// Failures returns the plugin's recent failures, newest first. A failure kept
// by a lerd from before the log file is the whole history until a new one.
func Failures(name string) []Failure {
	data, err := os.ReadFile(historyPath(name))
	if err != nil {
		if f, ok := LastFailure(name); ok {
			return []Failure{f}
		}
		return nil
	}
	var out []Failure
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var f Failure
		if json.Unmarshal([]byte(line), &f) == nil {
			out = append(out, f)
		}
	}
	slices.Reverse(out)
	return out
}

// historyMu and a lock file keep each read, trim and write of a history
// whole: the mutex between goroutines, flock between lerd processes.
var historyMu sync.Mutex

// appendHistory adds f to the plugin's history, one JSON object per line,
// dropping the oldest past failureHistory.
func appendHistory(f Failure) {
	if !nameRe.MatchString(f.Plugin) {
		return
	}
	line, err := json.Marshal(f)
	if err != nil {
		return
	}
	path := historyPath(f.Plugin)
	config.GuardRealWrite(path)
	if os.MkdirAll(failuresDir(), 0o755) != nil {
		return
	}
	historyMu.Lock()
	defer historyMu.Unlock()
	// The lock sits on a file of its own, since the history is replaced by a
	// rename and a lock on the old file would not cover the new one.
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX) != nil {
		return
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	lines := []string{}
	if data, err := os.ReadFile(path); err == nil {
		if text := strings.TrimSpace(string(data)); text != "" {
			lines = strings.Split(text, "\n")
		}
	}
	lines = append(lines, string(line))
	if len(lines) > failureHistory {
		lines = lines[len(lines)-failureHistory:]
	}
	// Written beside the log and renamed over it, so a reader sees the old
	// history or the new one, never half of it.
	tmp := path + ".tmp"
	if os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
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
