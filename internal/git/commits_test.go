package git

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// commitsFixture leaves dir one commit behind its upstream and two ahead of it.
func commitsFixture(t *testing.T) (remote, dir string) {
	t.Helper()
	remote, dir = t.TempDir(), t.TempDir()
	gitRun(t, remote, "init", "-q", "--bare")
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "one\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	gitRun(t, dir, "remote", "add", "origin", remote)
	gitRun(t, dir, "push", "-q", "-u", "origin", "main")

	other := t.TempDir()
	gitRun(t, other, "clone", "-q", remote, ".")
	write(t, filepath.Join(other, "theirs"), "t\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "commit", "-q", "-m", "their change")
	gitRun(t, other, "push", "-q")
	gitRun(t, dir, "fetch", "-q")

	write(t, filepath.Join(dir, "a"), "one\ntwo\n")
	gitRun(t, dir, "commit", "-q", "-am", "edit a")
	write(t, filepath.Join(dir, "b"), "bee\n")
	gitRun(t, dir, "add", "b")
	gitRun(t, dir, "commit", "-q", "-m", "add b")
	return remote, dir
}

func subjects(l CommitList) string {
	var s []string
	for _, c := range l.Commits {
		s = append(s, c.Subject)
	}
	return strings.Join(s, ",")
}

func TestReadCommits(t *testing.T) {
	_, dir := commitsFixture(t)
	out, err := ReadCommits(dir, CommitsOutgoing)
	if err != nil || subjects(out) != "add b,edit a" || out.More {
		t.Fatalf("outgoing: %+v %v", out, err)
	}
	if len(out.Commits[0].Hash) != 40 {
		t.Errorf("hash: %q", out.Commits[0].Hash)
	}
	if want := strings.TrimSpace(gitOut(t, dir, "log", "-1", "--format=%ct")); strconv.FormatInt(out.Commits[0].Time, 10) != want {
		t.Errorf("time: got %d want %s", out.Commits[0].Time, want)
	}
	in, err := ReadCommits(dir, CommitsIncoming)
	if err != nil || subjects(in) != "their change" {
		t.Fatalf("incoming: %+v %v", in, err)
	}
	if _, err := ReadCommits(dir, "HEAD~5..HEAD"); err == nil {
		t.Error("a free-form range must be refused")
	}
}

// A branch with no upstream publishes whatever no remote has yet.
func TestReadCommits_unpublishedBranch(t *testing.T) {
	_, dir := commitsFixture(t)
	gitRun(t, dir, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(dir, "c"), "c\n")
	gitRun(t, dir, "add", "c")
	gitRun(t, dir, "commit", "-q", "-m", "add c")
	out, err := ReadCommits(dir, CommitsOutgoing)
	if err != nil || subjects(out) != "add c,add b,edit a" {
		t.Fatalf("outgoing: %+v %v", out, err)
	}
	if _, err := ReadCommits(dir, CommitsIncoming); err == nil {
		t.Error("nothing comes in without an upstream")
	}
}

func TestReadCommits_cutLongList(t *testing.T) {
	_, dir := commitsFixture(t)
	defer func(n int) { maxCommits = n }(maxCommits)
	maxCommits = 1
	out, err := ReadCommits(dir, CommitsOutgoing)
	if err != nil || subjects(out) != "add b" || !out.More {
		t.Fatalf("cut: %+v %v", out, err)
	}
}

func TestCommitFilesAndVersions(t *testing.T) {
	_, dir := commitsFixture(t)
	gitRun(t, dir, "rm", "-q", "b")
	gitRun(t, dir, "commit", "-q", "-m", "drop b")
	commits := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(gitOut(t, dir, "log", "--format=%s\x1f%H")), "\n") {
		s, h, _ := strings.Cut(line, "\x1f")
		commits[s] = h
	}

	cases := []struct {
		subject, path, status, original, modified string
	}{
		{"base", "a", "A", "", "one\n"},
		{"edit a", "a", "M", "one\n", "one\ntwo\n"},
		{"add b", "b", "A", "", "bee\n"},
		{"drop b", "b", "D", "bee\n", ""},
	}
	for _, c := range cases {
		// A short hash, as a user would copy it, names the commit as well.
		sha := commits[c.subject][:10]
		files, err := CommitFiles(dir, sha)
		if err != nil || len(files) != 1 || files[0] != (ChangedFile{c.path, c.status}) {
			t.Fatalf("%s files: %+v %v", c.subject, files, err)
		}
		v, err := CommitVersions(dir, sha, c.path)
		if err != nil || v.Original != c.original || v.Modified != c.modified {
			t.Errorf("%s versions: %+v %v", c.subject, v, err)
		}
	}

	if _, err := CommitVersions(dir, commits["edit a"], "b"); err == nil {
		t.Error("a path the commit leaves alone must be refused")
	}
	for _, bad := range []string{"HEAD", "--output=/tmp/x", "main", "", "0000000000"} {
		if _, err := CommitFiles(dir, bad); err == nil {
			t.Errorf("%q: expected a refusal", bad)
		}
	}
}

// What a pull brings in, across every incoming commit, as its plan counts it.
func TestHeadFilesAndVersions(t *testing.T) {
	_, dir := commitsFixture(t)
	gitRun(t, dir, "reset", "-q", "--hard", "HEAD~2")
	upstream := strings.TrimSpace(gitOut(t, dir, "rev-parse", "@{upstream}"))

	files, err := HeadFiles(dir, upstream)
	if err != nil || len(files) != 1 || files[0] != (ChangedFile{"theirs", "A"}) {
		t.Fatalf("files: %+v %v", files, err)
	}
	if v, err := HeadVersions(dir, upstream, "theirs"); err != nil || v.Original != "" || v.Modified != "t\n" {
		t.Errorf("versions: %+v %v", v, err)
	}
	if _, err := HeadVersions(dir, upstream, "a"); err == nil {
		t.Error("a path the pull leaves alone must be refused")
	}
	if _, err := HeadFiles(dir, "@{upstream}"); err == nil {
		t.Error("only a hash may name the commit")
	}
}

// What a push sends, across every outgoing commit, leaving out what only the
// remote has.
func TestOutgoingFilesAndVersions(t *testing.T) {
	_, dir := commitsFixture(t)

	files, err := OutgoingFiles(dir)
	if err != nil || len(files) != 2 || files[0] != (ChangedFile{"a", "M"}) || files[1] != (ChangedFile{"b", "A"}) {
		t.Fatalf("files: %+v %v", files, err)
	}
	if v, err := OutgoingVersions(dir, "a"); err != nil || v.Original != "one\n" || v.Modified != "one\ntwo\n" {
		t.Errorf("versions: %+v %v", v, err)
	}
	if _, err := OutgoingVersions(dir, "theirs"); err == nil {
		t.Error("a path only the remote changed must be refused")
	}
	gitRun(t, dir, "checkout", "-q", "-b", "feature")
	if _, err := OutgoingFiles(dir); err == nil {
		t.Error("a branch with no upstream has nothing to compare with")
	}
}
