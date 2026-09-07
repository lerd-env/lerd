package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

func stubDomainSync(t *testing.T, err error) *[]string {
	t.Helper()
	var dirs []string
	prev := domainSyncEnvFn
	domainSyncEnvFn = func(dir string, _ io.Writer) error {
		dirs = append(dirs, dir)
		return err
	}
	t.Cleanup(func() { domainSyncEnvFn = prev })
	return &dirs
}

func siteUsingRustfs(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_ENDPOINT=http://lerd-rustfs:9000\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: name, Domains: []string{name + ".test"}, Path: dir}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	return dir
}

// A domain that never reaches the projects pointing at the service does
// nothing, and removing one would strand every project on a name that resolves
// nowhere. The sweep is what closes both.
func TestSyncServiceDomainSites_RewritesEverySiteUsingTheService(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	a := siteUsingRustfs(t, "shop")
	b := siteUsingRustfs(t, "blog")
	dirs := stubDomainSync(t, nil)

	syncServiceDomainSites("rustfs", config.SitesUsingService("rustfs"))

	if len(*dirs) != 2 {
		t.Fatalf("expected both sites swept, got %v", *dirs)
	}
	got := map[string]bool{(*dirs)[0]: true, (*dirs)[1]: true}
	if !got[a] || !got[b] {
		t.Errorf("expected %q and %q, got %v", a, b, *dirs)
	}
}

func TestSyncServiceDomainSites_NoSitesIsSilent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dirs := stubDomainSync(t, nil)

	syncServiceDomainSites("rustfs", config.SitesUsingService("rustfs"))

	if len(*dirs) != 0 {
		t.Errorf("expected no sweep with no sites, got %v", *dirs)
	}
}

// One project that cannot be written must not stop the rest: the others are
// still pointing at the old address until they are swept.
func TestSyncServiceDomainSites_ContinuesPastAFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	siteUsingRustfs(t, "shop")
	siteUsingRustfs(t, "blog")
	dirs := stubDomainSync(t, errors.New("boom"))

	syncServiceDomainSites("rustfs", config.SitesUsingService("rustfs"))

	if len(*dirs) != 2 {
		t.Errorf("expected both attempted, got %v", *dirs)
	}
}
