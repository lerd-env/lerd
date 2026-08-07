package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The image's sendmail is BusyBox's, which reaches 127.0.0.1:25 and nothing
// else, so the ini has to name the catcher or every mail() call fails.
func TestMailIni_pointsSendmailAtTheCatcher(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ini := MailIni()
	if !strings.Contains(ini, "sendmail_path") {
		t.Fatalf("ini sets no sendmail_path:\n%s", ini)
	}
	if !strings.Contains(ini, "-S lerd-mailpit:1025") {
		t.Errorf("ini does not route to the catcher:\n%s", ini)
	}
	// -t reads the recipients from the message and -i keeps a lone dot from
	// ending it, which is what PHP's own default passes.
	if !strings.Contains(ini, "-t -i") {
		t.Errorf("ini drops the flags PHP passes by default:\n%s", ini)
	}
}

// The ini is a bind-mount source, so it has to be a regular file: podman
// silently creates a directory in its place otherwise and the container comes
// up with a directory where php expects an ini.
func TestEnsureMailAssets_writesARegularFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := EnsureMailAssets(); err != nil {
		t.Fatalf("EnsureMailAssets: %v", err)
	}
	info, err := os.Stat(config.MailIniFile())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.IsDir() {
		t.Fatal("the ini path is a directory")
	}
	body, _ := os.ReadFile(config.MailIniFile())
	if !strings.Contains(string(body), "sendmail_path") {
		t.Errorf("written ini has no sendmail_path:\n%s", body)
	}
}

// A directory podman left at the mount source is healed rather than inherited.
func TestEnsureMailAssets_healsAStaleDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := os.MkdirAll(config.MailIniFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMailAssets(); err != nil {
		t.Fatalf("EnsureMailAssets: %v", err)
	}
	info, err := os.Stat(config.MailIniFile())
	if err != nil || info.IsDir() {
		t.Errorf("stale directory not healed: info=%v err=%v", info, err)
	}
}

// Rewriting an unchanged ini would churn a file mounted into every running
// container on every reconcile.
func TestEnsureMailAssets_skipsAnUnchangedWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := EnsureMailAssets(); err != nil {
		t.Fatalf("EnsureMailAssets: %v", err)
	}
	before, err := os.Stat(config.MailIniFile())
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureMailAssets(); err != nil {
		t.Fatalf("EnsureMailAssets: %v", err)
	}
	after, err := os.Stat(config.MailIniFile())
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged ini was rewritten")
	}
}

// The quadlet has to mount it, or the ini sits on the host doing nothing.
func TestFPMQuadlet_mountsTheMailIni(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	want := config.MailIniFile() + ":/usr/local/etc/php/conf.d/94-lerd-mail.ini:ro"
	if !strings.Contains(content, want) {
		t.Errorf("quadlet does not mount the mail ini (%s):\n%s", want, content)
	}
	// conf.d files load in name order, so 94 lands before the 95 shared file and
	// the 98 per-version one: a user setting their own sendmail_path in either
	// still wins over lerd's.
	if !strings.Contains(filepath.Base(config.MailIniFile()), "94-") {
		t.Errorf("the mail ini must sort below the shared and user files, got %s", config.MailIniFile())
	}
}
