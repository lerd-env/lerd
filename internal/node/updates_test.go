package node

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// serveReleases points the release index at a test server and counts its hits.
func serveReleases(t *testing.T, status int, body string) *atomic.Int32 {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	orig := releaseIndexURL
	t.Cleanup(func() { releaseIndexURL = orig })
	releaseIndexURL = srv.URL
	return &hits
}

const releaseIndex = `[
	{"version":"v27.0.0-alpha.0.0.0"},
	{"version":"v24.22.3"},
	{"version":"v24.9.0"},
	{"version":"v22.23.4"}
]`

func TestVersionStatuses_ReportsTheNewestInstalledPatchOfEachMajor(t *testing.T) {
	got := versionStatuses([]string{"22.23.2", "22.23.10", "24.22.2"}, map[string]string{"22": "22.23.10", "24": "24.22.3"})
	want := []VersionStatus{
		{Major: "22", Installed: "22.23.10", Latest: "22.23.10"},
		{Major: "24", Installed: "24.22.2", Latest: "24.22.3", UpdateAvailable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// With nothing to compare against there is no claim to make either way.
func TestVersionStatuses_OfflineMeansNoUpdateClaim(t *testing.T) {
	got := versionStatuses([]string{"24.22.2"}, nil)
	want := []VersionStatus{{Major: "24", Installed: "24.22.2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A nightly or a mirror lagging behind must not read as an update backwards.
func TestVersionStatuses_NewerThanPublishedIsNotAnUpdate(t *testing.T) {
	got := versionStatuses([]string{"24.23.0"}, map[string]string{"24": "24.22.9"})
	if got[0].UpdateAvailable {
		t.Fatalf("offered a downgrade: %+v", got[0])
	}
}

func TestVersionStatuses_IsNeverNil(t *testing.T) {
	if got := versionStatuses(nil, nil); got == nil {
		t.Fatal("got nil, which would reach the UI as null")
	}
}

func TestLatestByMajor_TakesTheNewestStableOfEachMajor(t *testing.T) {
	serveReleases(t, http.StatusOK, releaseIndex)

	got := latestByMajor(context.Background())
	want := map[string]string{"24": "24.22.3", "22": "22.23.4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLatestByMajor_AsksOncePerDay(t *testing.T) {
	hits := serveReleases(t, http.StatusOK, releaseIndex)

	latestByMajor(context.Background())
	latestByMajor(context.Background())
	if n := hits.Load(); n != 1 {
		t.Fatalf("fetched %d times, want once", n)
	}
}

// Last week's answer beats no answer, and a dead network is not retried on
// every page load.
func TestLatestByMajor_KeepsTheLastAnswerWhenTheFetchFails(t *testing.T) {
	hits := serveReleases(t, http.StatusBadGateway, "")
	stale, _ := json.Marshal(releaseCache{CheckedAt: time.Now().Add(-48 * time.Hour), Latest: map[string]string{"24": "24.22.2"}})
	if err := os.MkdirAll(config.CacheDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(releaseCacheFile(), stale, 0o644); err != nil {
		t.Fatal(err)
	}

	first := latestByMajor(context.Background())
	second := latestByMajor(context.Background())
	if first["24"] != "24.22.2" || second["24"] != "24.22.2" {
		t.Fatalf("lost the cached answer: %v, %v", first, second)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("fetched %d times after a failure, want once", n)
	}
}

func TestLatestByMajor_GarbageIsAFailureNotAnEmptyAnswer(t *testing.T) {
	serveReleases(t, http.StatusOK, "<html>captive portal</html>")

	if got := latestByMajor(context.Background()); len(got) != 0 {
		t.Fatalf("got %v from a page that is not the index", got)
	}
}

// fakeManager records what an update asks of the manager.
type fakeManager struct {
	Manager
	calls      []string
	installErr error
}

func (f *fakeManager) Install(v string) error {
	f.calls = append(f.calls, "install "+v)
	return f.installErr
}

func (f *fakeManager) SetDefault(v string) error {
	f.calls = append(f.calls, "default "+v)
	return nil
}

var outdated = VersionStatus{Major: "24", Installed: "24.22.2", Latest: "24.22.3", UpdateAvailable: true}

func TestApplyUpdate_InstallsTheExactPublishedPatch(t *testing.T) {
	m := &fakeManager{}
	got, err := applyUpdate(m, outdated, "22")
	if err != nil || got != "24.22.3" {
		t.Fatalf("got %q, %v", got, err)
	}
	if want := []string{"install 24.22.3"}; !reflect.DeepEqual(m.calls, want) {
		t.Errorf("calls = %v, want %v", m.calls, want)
	}
}

// fnm's default alias names one exact version, so without this the default
// stays on the old patch after the update.
func TestApplyUpdate_RepointsTheDefaultWhenThisMajorIsIt(t *testing.T) {
	m := &fakeManager{}
	if _, err := applyUpdate(m, outdated, "24"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"install 24.22.3", "default 24"}; !reflect.DeepEqual(m.calls, want) {
		t.Errorf("calls = %v, want %v", m.calls, want)
	}
}

func TestApplyUpdate_LeavesAnExactDefaultPinAlone(t *testing.T) {
	m := &fakeManager{}
	if _, err := applyUpdate(m, outdated, "24.21.0"); err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 1 {
		t.Errorf("touched more than the install: %v", m.calls)
	}
}

func TestApplyUpdate_CurrentVersionDoesNothing(t *testing.T) {
	m := &fakeManager{}
	got, err := applyUpdate(m, VersionStatus{Major: "24", Installed: "24.22.3", Latest: "24.22.3"}, "24")
	if err != nil || got != "24.22.3" || len(m.calls) != 0 {
		t.Fatalf("got %q, %v, calls %v", got, err, m.calls)
	}
}

func TestApplyUpdate_FailedInstallKeepsTheDefaultWhereItWas(t *testing.T) {
	m := &fakeManager{installErr: errors.New("download failed")}
	if _, err := applyUpdate(m, outdated, "24"); err == nil {
		t.Fatal("a failed install was reported as success")
	}
	if len(m.calls) != 1 {
		t.Errorf("repointed the default after a failed install: %v", m.calls)
	}
}

func TestCheckReleases_AsksEvenWhenTheCacheIsFresh(t *testing.T) {
	hits := serveReleases(t, http.StatusOK, releaseIndex)
	latestByMajor(context.Background())

	if err := CheckReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("fetched %d times, want the check to reach the network", n)
	}
}

func TestCheckReleases_FailureKeepsTheLastAnswer(t *testing.T) {
	serveReleases(t, http.StatusBadGateway, "")
	fresh, _ := json.Marshal(releaseCache{CheckedAt: time.Now(), Latest: map[string]string{"24": "24.22.2"}})
	if err := os.MkdirAll(config.CacheDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(releaseCacheFile(), fresh, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CheckReleases(context.Background()); err == nil {
		t.Fatal("a failed check was reported as success")
	}
	if got := latestByMajor(context.Background()); got["24"] != "24.22.2" {
		t.Fatalf("lost the cached answer: %v", got)
	}
}
