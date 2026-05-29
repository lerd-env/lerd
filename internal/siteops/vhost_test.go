package siteops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func seedFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestMoveCustomNginxConfig_movesLiveOverrideAndBackups(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	live := config.NginxCustomD()
	bkp := config.NginxCustomDBkp()
	seedFile(t, filepath.Join(live, "scorediviner.test.conf"), "client_max_body_size 200m;\n")
	seedFile(t, filepath.Join(bkp, "scorediviner.test.conf.bkp.20260101-101010"), "old1\n")
	seedFile(t, filepath.Join(bkp, "scorediviner.test.conf.bkp.20260101-101010-1"), "old2\n")
	seedFile(t, filepath.Join(bkp, "other.test.conf.bkp.20260101-101010"), "unrelated\n")

	if err := MoveCustomNginxConfig("scorediviner.test", "therealscore.test"); err != nil {
		t.Fatalf("MoveCustomNginxConfig: %v", err)
	}

	if _, err := os.Stat(filepath.Join(live, "scorediviner.test.conf")); !os.IsNotExist(err) {
		t.Errorf("old live override still present, want removed")
	}
	body, err := os.ReadFile(filepath.Join(live, "therealscore.test.conf"))
	if err != nil || string(body) != "client_max_body_size 200m;\n" {
		t.Errorf("new live override = %q, err=%v; want original content under new name", body, err)
	}

	for _, suffix := range []string{"20260101-101010", "20260101-101010-1"} {
		if _, err := os.Stat(filepath.Join(bkp, "scorediviner.test.conf.bkp."+suffix)); !os.IsNotExist(err) {
			t.Errorf("old backup %s still present, want renamed", suffix)
		}
		if _, err := os.Stat(filepath.Join(bkp, "therealscore.test.conf.bkp."+suffix)); err != nil {
			t.Errorf("new backup %s missing: %v", suffix, err)
		}
	}

	if _, err := os.Stat(filepath.Join(bkp, "other.test.conf.bkp.20260101-101010")); err != nil {
		t.Errorf("unrelated site's backup was disturbed: %v", err)
	}
}

func TestMoveCustomNginxConfig_noOverrideIsNoError(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := MoveCustomNginxConfig("scorediviner.test", "therealscore.test"); err != nil {
		t.Fatalf("MoveCustomNginxConfig with no files present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.NginxCustomD(), "therealscore.test.conf")); !os.IsNotExist(err) {
		t.Errorf("a new override was fabricated where none existed")
	}
}

func TestMoveCustomNginxConfig_liveOverrideReplacesStaleOrphan(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	live := config.NginxCustomD()
	seedFile(t, filepath.Join(live, "scorediviner.test.conf"), "current config\n")
	seedFile(t, filepath.Join(live, "therealscore.test.conf"), "stale orphan\n")

	if err := MoveCustomNginxConfig("scorediviner.test", "therealscore.test"); err != nil {
		t.Fatalf("MoveCustomNginxConfig: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(live, "therealscore.test.conf"))
	if err != nil || string(body) != "current config\n" {
		t.Errorf("new live override = %q err=%v; want stale orphan replaced by current config", body, err)
	}
}

func TestMoveCustomNginxConfig_doesNotClobberExistingBackup(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	bkp := config.NginxCustomDBkp()
	seedFile(t, filepath.Join(bkp, "scorediviner.test.conf.bkp.20260101-101010"), "source\n")
	seedFile(t, filepath.Join(bkp, "therealscore.test.conf.bkp.20260101-101010"), "existing\n")

	if err := MoveCustomNginxConfig("scorediviner.test", "therealscore.test"); err != nil {
		t.Fatalf("MoveCustomNginxConfig: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(bkp, "therealscore.test.conf.bkp.20260101-101010"))
	if err != nil || string(body) != "existing\n" {
		t.Errorf("colliding backup = %q err=%v; want existing history preserved, not clobbered", body, err)
	}
	if _, err := os.Stat(filepath.Join(bkp, "scorediviner.test.conf.bkp.20260101-101010")); err != nil {
		t.Errorf("source backup should be left in place when destination collides: %v", err)
	}
}

func TestMoveCustomNginxConfig_samePrimaryIsNoop(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	live := config.NginxCustomD()
	seedFile(t, filepath.Join(live, "scorediviner.test.conf"), "keep me\n")

	if err := MoveCustomNginxConfig("scorediviner.test", "scorediviner.test"); err != nil {
		t.Fatalf("MoveCustomNginxConfig noop: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(live, "scorediviner.test.conf"))
	if err != nil || string(body) != "keep me\n" {
		t.Errorf("override disturbed by same-domain call: %q err=%v", body, err)
	}
}
