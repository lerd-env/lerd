package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func changesFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "one\n")
	write(t, filepath.Join(dir, "b"), "two\n")
	write(t, filepath.Join(dir, ".gitignore"), "ignored\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	write(t, filepath.Join(dir, "a"), "one staged\n")
	gitRun(t, dir, "add", "a")
	write(t, filepath.Join(dir, "a"), "one staged then edited\n")
	gitRun(t, dir, "rm", "-q", "b")
	write(t, filepath.Join(dir, "new file.txt"), "fresh\n")
	write(t, filepath.Join(dir, "ignored"), "secret\n")
	return dir
}

func TestReadChanges(t *testing.T) {
	dir := changesFixture(t)
	got, err := ReadChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	same := func(name string, got, want []ChangedFile) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s[%d]: got %v want %v", name, i, got[i], want[i])
			}
		}
	}
	same("staged", got.Staged, []ChangedFile{{"a", "M"}, {"b", "D"}})
	same("unstaged", got.Unstaged, []ChangedFile{{"a", "M"}})
	same("untracked", got.Untracked, []ChangedFile{{"new file.txt", "?"}})
}

// Reading a working file takes whatever path it gets, so only listed paths may reach it.
func TestFileVersions_refusesUnlistedPath(t *testing.T) {
	dir := changesFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	write(t, outside, "x")
	for _, c := range []struct{ section, path string }{
		{SectionUntracked, outside},
		{SectionUntracked, "../outside"},
		{SectionUntracked, "ignored"},
		{SectionUnstaged, "b"},
		{"everything", "a"},
	} {
		if _, err := FileVersions(dir, c.section, c.path); err == nil {
			t.Errorf("%s %s: expected a refusal", c.section, c.path)
		}
	}
}

// The dashboard's own "Initialize git" leaves a repo with no commit yet.
func TestReadChanges_beforeFirstCommit(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "staged.php"), "s\n")
	write(t, filepath.Join(dir, "loose.php"), "l\n")
	gitRun(t, dir, "add", "staged.php")

	got, err := ReadChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Staged) != 1 || got.Staged[0] != (ChangedFile{"staged.php", "A"}) || len(got.Untracked) != 1 {
		t.Fatalf("got %+v", got)
	}
	v, err := FileVersions(dir, SectionStaged, "staged.php")
	if err != nil || v.Original != "" || v.Modified != "s\n" {
		t.Fatalf("versions %+v err %v", v, err)
	}
}

// Each section compares its own pair: HEAD to index, index to tree, nothing to tree.
func TestFileVersions(t *testing.T) {
	dir := changesFixture(t)
	cases := []struct{ section, path, original, modified string }{
		{SectionStaged, "a", "one\n", "one staged\n"},
		{SectionUnstaged, "a", "one staged\n", "one staged then edited\n"},
		{SectionStaged, "b", "two\n", ""},
		{SectionUntracked, "new file.txt", "", "fresh\n"},
	}
	for _, c := range cases {
		v, err := FileVersions(dir, c.section, c.path)
		if err != nil {
			t.Fatalf("%s %s: %v", c.section, c.path, err)
		}
		if v.Original != c.original || v.Modified != c.modified {
			t.Errorf("%s %s: got %q -> %q", c.section, c.path, v.Original, v.Modified)
		}
	}
}

func TestFileVersions_linkBinaryAndHuge(t *testing.T) {
	dir := changesFixture(t)
	secret := filepath.Join(t.TempDir(), "secret")
	write(t, secret, "do not read")
	if err := os.Symlink(secret, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "logo.png"), "PNG\x00\x01")
	write(t, filepath.Join(dir, "bundle.js"), strings.Repeat("x", maxVersionBytes+1))

	if v, _ := FileVersions(dir, SectionUntracked, "link"); v.Modified != secret {
		t.Errorf("symlink: got %q, want its target path", v.Modified)
	}
	if v, _ := FileVersions(dir, SectionUntracked, "logo.png"); !v.Binary || v.Modified != "" {
		t.Errorf("binary: %+v", v)
	}
	if v, _ := FileVersions(dir, SectionUntracked, "bundle.js"); !v.TooLarge || v.Modified != "" {
		t.Errorf("huge: too_large=%v len=%d", v.TooLarge, len(v.Modified))
	}
}

// A dump left in the checkout is refused on its size, never read: the sparse
// file below would take minutes and gigabytes of memory to load.
func TestFileVersions_refusesHugeFilesBeforeReading(t *testing.T) {
	dir := changesFixture(t)
	dump := filepath.Join(dir, "dump.sql")
	write(t, dump, "")
	if err := os.Truncate(dump, 8<<30); err != nil {
		t.Skip("no sparse files here:", err)
	}
	if v, err := FileVersions(dir, SectionUntracked, "dump.sql"); err != nil || !v.TooLarge {
		t.Fatalf("untracked dump: %+v %v", v, err)
	}

	big := strings.Repeat("x", maxVersionBytes+1)
	write(t, filepath.Join(dir, "big.js"), big)
	gitRun(t, dir, "add", "big.js")
	write(t, filepath.Join(dir, "big.js"), "small now")
	if v, err := FileVersions(dir, SectionUnstaged, "big.js"); err != nil || !v.TooLarge {
		t.Fatalf("indexed blob over the limit: %+v %v", v, err)
	}
}

// A conflict has no single index entry and a submodule no file; showing either
// as wholly added or deleted would be a lie.
func TestFileVersions_unreadableSides(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "c"), "base\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	gitRun(t, dir, "checkout", "-q", "-b", "other")
	write(t, filepath.Join(dir, "c"), "theirs\n")
	gitRun(t, dir, "commit", "-q", "-am", "theirs")
	gitRun(t, dir, "checkout", "-q", "main")
	write(t, filepath.Join(dir, "c"), "ours\n")
	gitRun(t, dir, "commit", "-q", "-am", "ours")
	gitRun(t, dir, "merge", "-q", "other")

	// git diff lists a conflicted path twice, as U and as M; the dashboard keys rows by path.
	got, err := ReadChanges(dir)
	if err != nil || len(got.Unstaged) != 1 || got.Unstaged[0] != (ChangedFile{"c", "U"}) {
		t.Fatalf("conflict listed as %+v %v", got.Unstaged, err)
	}
	if v, err := FileVersions(dir, SectionUnstaged, "c"); err != nil || !v.Unreadable || v.Modified != "" {
		t.Errorf("conflict: %+v %v", v, err)
	}

	sub := t.TempDir()
	gitRun(t, sub, "init", "-q")
	gitRun(t, sub, "commit", "-q", "--allow-empty", "-m", "sub")
	head := strings.TrimSpace(gitOut(t, sub, "rev-parse", "HEAD"))
	gitRun(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",vendor/sub")
	if v, err := FileVersions(dir, SectionStaged, "vendor/sub"); err != nil || !v.Unreadable {
		t.Errorf("submodule: %+v %v", v, err)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := Output(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}
