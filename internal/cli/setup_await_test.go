package cli

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A setup step waits for its PHP and starts it when it is down, instead of
// failing on podman's bare exit 125.
func TestSetupExecsWaitForTheContainer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // a regression must not reach the real podman
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}, PHPVersion: "8.3"}); err != nil {
		t.Fatal(err)
	}
	prev := awaitContainerFn
	t.Cleanup(func() { awaitContainerFn = prev })
	wantErr := errors.New("not up")
	var waited []string
	awaitContainerFn = func(_, container string) error { waited = append(waited, container); return wantErr }

	if err := execInContainer(dir, "php artisan storage:link"); !errors.Is(err, wantErr) {
		t.Errorf("execInContainer err = %v, want the wait's", err)
	}
	if err := composerInContainer(dir, "install"); !errors.Is(err, wantErr) {
		t.Errorf("composerInContainer err = %v, want the wait's", err)
	}
	if len(waited) != 2 || waited[0] != "lerd-php83-fpm" || waited[1] != "lerd-php83-fpm" {
		t.Errorf("waited on %v, want lerd-php83-fpm twice", waited)
	}
}
