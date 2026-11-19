package podman

import (
	"testing"
	"time"
)

func TestNginxBehind(t *testing.T) {
	running := nginxLocal{RunningID: "aaa", LocalID: "aaa", RunningDigests: []string{"sha256:old", "sha256:index"}}
	cases := []struct {
		name   string
		local  nginxLocal
		latest string
		want   bool
	}{
		{"same digest upstream", running, "sha256:index", false},
		{"digest case differs", running, "SHA256:INDEX", false},
		{"upstream republished the tag", running, "sha256:new", true},
		{"registry unreachable is not an update", running, "", false},
		{"pulled but never restarted", nginxLocal{RunningID: "aaa", LocalID: "bbb", RunningDigests: running.RunningDigests}, "", true},
		{"nginx not running", nginxLocal{LocalID: "aaa"}, "sha256:new", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nginxBehind(c.local, c.latest); got != c.want {
				t.Errorf("nginxBehind = %v, want %v", got, c.want)
			}
		})
	}
}

// stubNginxRead fakes what podman and the digest cache report, so the status
// read is exercised without either.
func stubNginxRead(t *testing.T, local nginxLocal, cached string, ok bool) {
	t.Helper()
	prevLocal, prevCached, prevFetch := nginxLocalFn, cachedManifestDigestFn, manifestDigestFn
	nginxLocalFn = func() nginxLocal { return local }
	cachedManifestDigestFn = func(string) (string, bool) { return cached, ok }
	manifestDigestFn = func(string) (string, error) { return "", nil }
	t.Cleanup(func() {
		nginxRefreshWG.Wait()
		nginxLocalFn, cachedManifestDigestFn, manifestDigestFn = prevLocal, prevCached, prevFetch
	})
}

func TestNginxUpdateAvailable_ColdCacheIsNotAnUpdate(t *testing.T) {
	stubNginxRead(t, nginxLocal{RunningID: "aaa", LocalID: "aaa", RunningDigests: []string{"sha256:old"}}, "", false)
	if NginxUpdateAvailable() {
		t.Error("a digest nobody has fetched yet must not read as an update")
	}
}

func TestNginxUpdateAvailable_RepublishedTag(t *testing.T) {
	stubNginxRead(t, nginxLocal{RunningID: "aaa", LocalID: "aaa", RunningDigests: []string{"sha256:old"}}, "sha256:new", true)
	if !NginxUpdateAvailable() {
		t.Error("a cached digest the running image lacks must read as an update")
	}
}

// A restart onto another image by anyone (systemctl, lerd install, a manual
// podman tag) must show on the very next read, not after a cache window.
func TestNginxUpdateAvailable_SeesARestartImmediately(t *testing.T) {
	stubNginxRead(t, nginxLocal{RunningID: "new", LocalID: "new", RunningDigests: []string{"sha256:latest"}}, "sha256:latest", true)
	if NginxUpdateAvailable() {
		t.Fatal("current image read as behind")
	}
	nginxLocalFn = func() nginxLocal {
		return nginxLocal{RunningID: "old", LocalID: "old", RunningDigests: []string{"sha256:older"}}
	}
	if !NginxUpdateAvailable() {
		t.Error("nginx restarted onto an older image, but the read still reports it current")
	}
}

// The background fetch is what notices a republished tag, so finishing it has
// to wake the status broadcast; otherwise the update waits on an unrelated event.
func TestNginxDigestRefreshAnnouncesItself(t *testing.T) {
	prevFetch, prevHook := manifestDigestFn, onNginxDigestRefreshed
	t.Cleanup(func() {
		nginxRefreshWG.Wait()
		manifestDigestFn, onNginxDigestRefreshed = prevFetch, prevHook
	})
	manifestDigestFn = func(string) (string, error) { return "sha256:new", nil }
	done := make(chan struct{})
	SetOnNginxDigestRefreshed(func() { close(done) })

	scheduleNginxDigestRefresh("docker.io/library/nginx:alpine")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a finished digest fetch did not announce itself")
	}
}

// Podman records a republished tag's new index digest on the image ID it
// already had. The pull may run in another process (the CLI, the TUI, lerd
// install), so lerd-ui's next read must see it with nothing cleared here.
func TestReadNginxLocal_SeesDigestsAPullAddedElsewhere(t *testing.T) {
	prevC, prevL, prevD := containerImageIDFn, localImageIDFn, localImageDigestFn
	t.Cleanup(func() { containerImageIDFn, localImageIDFn, localImageDigestFn = prevC, prevL, prevD })
	containerImageIDFn = func(string) string { return "aaa" }
	localImageIDFn = func(string) string { return "aaa" }
	localImageDigestFn = func(string) []string { return []string{"sha256:old"} }
	readNginxLocal()

	localImageDigestFn = func(string) []string { return []string{"sha256:old", "sha256:new"} }
	if got := readNginxLocal(); nginxBehind(got, "sha256:new") {
		t.Errorf("digests read = %v, still behind after another process pulled sha256:new", got.RunningDigests)
	}
}
