package siteops

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
)

type secureStubs struct {
	secureCallCount   int
	unsecureCallCount int
	reloadCallCount   int
	secureErr         error
	unsecureErr       error
	reloadErr         error
	notifications     []string // "domain:action" entries in call order
}

// stubSecureDeps replaces every external dependency SetSecured touches so
// tests run without mkcert, podman, nginx, or the daemon HTTP API.
func stubSecureDeps(t *testing.T) *secureStubs {
	t.Helper()
	s := &secureStubs{}
	origSecure := secureCertFn
	origUnsecure := unsecureCertFn
	origReload := nginxReloadFn
	origNotify := notifyDaemonFn
	secureCertFn = func(_ config.Site) error {
		s.secureCallCount++
		return s.secureErr
	}
	unsecureCertFn = func(_ config.Site) error {
		s.unsecureCallCount++
		return s.unsecureErr
	}
	nginxReloadFn = func() error {
		s.reloadCallCount++
		return s.reloadErr
	}
	notifyDaemonFn = func(domain, action string) error {
		s.notifications = append(s.notifications, domain+":"+action)
		return nil
	}
	t.Cleanup(func() {
		secureCertFn = origSecure
		unsecureCertFn = origUnsecure
		nginxReloadFn = origReload
		notifyDaemonFn = origNotify
	})
	return s
}

func withTempEnv(t *testing.T) (projectDir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	projectDir = t.TempDir()
	envPath := filepath.Join(projectDir, ".env")
	body := "APP_URL=http://myapp.test\n"
	if err := os.WriteFile(envPath, []byte(body), 0644); err != nil {
		t.Fatalf("seed .env: %v", err)
	}
	return projectDir
}

func TestSetSecured_securingCallsSecureSiteAndFlipsFlag(t *testing.T) {
	stubs := stubSecureDeps(t)
	projectDir := withTempEnv(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir}
	if err := config.AddSite(*site); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	if err := SetSecured(site, true); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}

	if stubs.secureCallCount != 1 {
		t.Errorf("certs.SecureSite calls = %d, want 1", stubs.secureCallCount)
	}
	if stubs.unsecureCallCount != 0 {
		t.Errorf("certs.UnsecureSite calls = %d, want 0", stubs.unsecureCallCount)
	}
	if stubs.reloadCallCount != 1 {
		t.Errorf("nginx.Reload calls = %d, want 1", stubs.reloadCallCount)
	}
	if !site.Secured {
		t.Errorf("site.Secured = false after secure, want true")
	}
	reg, err := config.FindSite("myapp")
	if err != nil || !reg.Secured {
		t.Errorf("registry secured flag not persisted; err=%v reg=%+v", err, reg)
	}
	body, _ := os.ReadFile(filepath.Join(projectDir, ".env"))
	if !strings.Contains(string(body), "APP_URL=https://myapp.test") {
		t.Errorf(".env APP_URL not flipped to https:\n%s", body)
	}
}

func TestSetSecured_unsecuringCallsUnsecureSiteAndFlipsFlag(t *testing.T) {
	stubs := stubSecureDeps(t)
	projectDir := withTempEnv(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir, Secured: true}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	if err := SetSecured(site, false); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}

	if stubs.unsecureCallCount != 1 || stubs.secureCallCount != 0 {
		t.Errorf("call counts: secure=%d unsecure=%d, want secure=0 unsecure=1",
			stubs.secureCallCount, stubs.unsecureCallCount)
	}
	if site.Secured {
		t.Errorf("site.Secured = true after unsecure, want false")
	}
}

func TestSetSecured_notifiesDaemonForStripeAndLANShare(t *testing.T) {
	// Every successful toggle must notify the daemon to refresh both
	// dependent listeners. Missing either has been the source of past bugs
	// (Stripe webhook stuck on wrong scheme, LAN share proxying to old
	// port). Pinning the call set + ordering catches future regressions.
	stubs := stubSecureDeps(t)
	projectDir := withTempEnv(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	if err := SetSecured(site, true); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}

	want := []string{"myapp.test:stripe:refresh", "myapp.test:lan:refresh"}
	if !equalStrings(stubs.notifications, want) {
		got := append([]string(nil), stubs.notifications...)
		sort.Strings(got)
		t.Errorf("daemon notifications = %v, want %v (order matters)", stubs.notifications, want)
	}
}

func TestSetSecured_skipsNotificationsAndAbortsOnCertError(t *testing.T) {
	stubs := stubSecureDeps(t)
	stubs.secureErr = errors.New("mkcert boom")
	projectDir := withTempEnv(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	err := SetSecured(site, true)
	if err == nil || !strings.Contains(err.Error(), "mkcert boom") {
		t.Errorf("expected cert error, got %v", err)
	}
	if site.Secured {
		t.Errorf("site.Secured should not have flipped after cert failure")
	}
	if len(stubs.notifications) != 0 {
		t.Errorf("daemon notifications fired after cert failure: %v", stubs.notifications)
	}
	if stubs.reloadCallCount != 0 {
		t.Errorf("nginx.Reload should not run after cert failure")
	}
}

func TestSetSecured_skipsNotificationsOnNginxReloadError(t *testing.T) {
	stubs := stubSecureDeps(t)
	stubs.reloadErr = errors.New("nginx down")
	projectDir := withTempEnv(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	if err := SetSecured(site, true); err == nil {
		t.Fatal("expected nginx reload error, got nil")
	}
	if len(stubs.notifications) != 0 {
		t.Errorf("daemon notifications fired after nginx reload failure: %v", stubs.notifications)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Silence unused-import warning if certs becomes irrelevant after stubbing.
var _ = certs.SecureSite
