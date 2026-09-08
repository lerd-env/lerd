package config

import (
	"os"
	"path/filepath"
	"testing"
)

func regOf(names ...string) *SiteRegistry {
	reg := &SiteRegistry{}
	for _, n := range names {
		reg.Sites = append(reg.Sites, Site{Name: n, Domains: []string{n + ".test"}, Path: "/srv/" + n})
	}
	return reg
}

func backupNames(t *testing.T) []string {
	t.Helper()
	list, err := ListSitesBackups()
	if err != nil {
		t.Fatalf("ListSitesBackups: %v", err)
	}
	out := make([]string, len(list))
	for i, b := range list {
		out[i] = b.Name
	}
	return out
}

// The first save has nothing to preserve, and every later save keeps the
// contents it is about to overwrite.
func TestSaveSitesKeepsPreviousContents(t *testing.T) {
	setDataDir(t)

	if err := SaveSites(regOf("one")); err != nil {
		t.Fatal(err)
	}
	if got := backupNames(t); len(got) != 0 {
		t.Fatalf("first save should not back anything up, got %v", got)
	}

	if err := SaveSites(regOf("one", "two")); err != nil {
		t.Fatal(err)
	}
	list, err := ListSitesBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 backup, got %d", len(list))
	}
	if list[0].Sites != 1 {
		t.Errorf("backup should hold the pre-save registry, got %d sites", list[0].Sites)
	}

	reg, err := ReadSitesBackup(list[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Sites) != 1 || reg.Sites[0].Name != "one" {
		t.Errorf("backup contents = %+v", reg.Sites)
	}
}

// The registry is rewritten constantly (idle state, worker toggles, version
// refreshes). A save that changes nothing must not push a real edit out of the
// window.
func TestSaveSitesSkipsUnchangedWrites(t *testing.T) {
	setDataDir(t)

	if err := SaveSites(regOf("one")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := SaveSites(regOf("one")); err != nil {
			t.Fatal(err)
		}
	}
	if got := backupNames(t); len(got) != 0 {
		t.Fatalf("identical saves should not create backups, got %v", got)
	}
}

func TestSaveSitesRotatesBackups(t *testing.T) {
	setDataDir(t)

	for i := 0; i <= sitesBackupKeep+3; i++ {
		if err := SaveSites(regOf(names(i)...)); err != nil {
			t.Fatal(err)
		}
	}
	list, err := ListSitesBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != sitesBackupKeep {
		t.Fatalf("want %d backups, got %d", sitesBackupKeep, len(list))
	}
	// Newest first, and the newest holds the registry the last save replaced.
	if list[0].Sites != sitesBackupKeep+2 {
		t.Errorf("newest backup has %d sites, want %d", list[0].Sites, sitesBackupKeep+2)
	}
	if list[0].Name <= list[len(list)-1].Name {
		t.Errorf("backups are not newest first: %v", backupNames(t))
	}
}

// The wipe this exists for: the registry is emptied and keeps being rewritten
// empty. The last good state must still be in the window.
func TestEmptySavesDoNotFlushTheWindow(t *testing.T) {
	setDataDir(t)

	if err := SaveSites(regOf(names(20)...)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := SaveSites(&SiteRegistry{}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := ListSitesBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Sites != 20 {
		t.Fatalf("want the 20-site registry preserved, got %+v", list)
	}
}

func TestRestoreSitesBackup(t *testing.T) {
	setDataDir(t)

	if err := SaveSites(regOf("one", "two")); err != nil {
		t.Fatal(err)
	}
	if err := SaveSites(&SiteRegistry{}); err != nil {
		t.Fatal(err)
	}

	restored, name, err := RestoreSitesBackup("")
	if err != nil {
		t.Fatalf("RestoreSitesBackup: %v", err)
	}
	if name == "" {
		t.Error("restore should report which backup it used")
	}
	if len(restored.Sites) != 2 {
		t.Fatalf("restored %d sites, want 2", len(restored.Sites))
	}
	live, err := LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Sites) != 2 {
		t.Fatalf("sites.yaml holds %d sites after restore, want 2", len(live.Sites))
	}
	// The registry the restore replaced is itself backed up, so a restore of
	// the wrong backup can be undone.
	list, err := ListSitesBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || list[0].Sites != 0 {
		t.Errorf("restore should back up the emptied registry, got %+v", list)
	}
}

func TestRestoreSitesBackupRejectsUnknownName(t *testing.T) {
	setDataDir(t)

	if _, _, err := RestoreSitesBackup("sites-20260101-000000.yaml"); err == nil {
		t.Error("restoring a missing backup should fail")
	}
	if _, _, err := RestoreSitesBackup("../../sites.yaml"); err == nil {
		t.Error("a traversing name should be refused")
	}
	if _, err := os.Stat(filepath.Join(SitesBackupDir())); err == nil {
		t.Error("a failed restore should not create the backup dir")
	}
}

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "s" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return out
}
