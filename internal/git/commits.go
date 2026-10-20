package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Directions a commit list can be read in, relative to the branch's upstream.
const (
	CommitsIncoming = "incoming"
	CommitsOutgoing = "outgoing"
)

// maxCommits bounds the list a confirmation shows; a variable so tests can cut it.
var maxCommits = 200

// Commit is one commit as a pull or push confirmation lists it.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	// Time is when it was committed, in Unix seconds.
	Time int64 `json:"time"`
}

// CommitList is newest first; More says commits past maxCommits were left out.
type CommitList struct {
	Commits []Commit `json:"commits"`
	More    bool     `json:"more"`
}

// ReadCommits lists what a pull would bring in or a push would send. A branch
// with no upstream sends whatever no remote has yet, the way publishing it does.
func ReadCommits(dir, direction string) (CommitList, error) {
	_, upErr := Output(dir, "rev-parse", "--verify", "--quiet", "@{upstream}")
	var revs []string
	switch {
	case direction == CommitsIncoming && upErr == nil:
		revs = []string{"HEAD..@{upstream}"}
	case direction == CommitsIncoming:
		return CommitList{}, fmt.Errorf("the branch tracks no upstream")
	case direction == CommitsOutgoing && upErr == nil:
		revs = []string{"@{upstream}..HEAD"}
	case direction == CommitsOutgoing:
		revs = []string{"HEAD", "--not", "--remotes"}
	default:
		return CommitList{}, fmt.Errorf("unknown commit direction %q", direction)
	}
	args := append([]string{"log", "-z", "--format=%H%x1f%ct%x1f%s", fmt.Sprintf("--max-count=%d", maxCommits+1)}, revs...)
	out, err := Output(dir, args...)
	if err != nil {
		return CommitList{}, err
	}
	list := CommitList{Commits: []Commit{}}
	for _, rec := range strings.Split(out, "\x00") {
		parts := strings.SplitN(strings.TrimPrefix(rec, "\n"), "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		when, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return CommitList{}, fmt.Errorf("commit %s: bad time %q", parts[0], parts[1])
		}
		if len(list.Commits) == maxCommits {
			list.More = true
			break
		}
		list.Commits = append(list.Commits, Commit{Hash: parts[0], Subject: parts[2], Time: when})
	}
	return list, nil
}

var hexHash = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// resolveCommit takes only a hash, so no ref name or option ever reaches git.
func resolveCommit(dir, hash string) (string, error) {
	if !hexHash.MatchString(hash) {
		return "", fmt.Errorf("%q is not a commit hash", hash)
	}
	out, err := Output(dir, "rev-parse", "--verify", "--quiet", hash+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("no commit %s here", hash)
	}
	return strings.TrimSpace(out), nil
}

// commitSpan compares a commit with its first parent, so a merge lists what
// it brought into the branch; a root commit has no parent and from is "".
func commitSpan(dir, hash string) (from, to string, err error) {
	if to, err = resolveCommit(dir, hash); err != nil {
		return "", "", err
	}
	if out, err := Output(dir, "rev-parse", "--verify", "--quiet", to+"^1"); err == nil {
		from = strings.TrimSpace(out)
	}
	return from, to, nil
}

// headSpan compares HEAD with a commit, the way a pull plan counts its files.
func headSpan(dir, hash string) (from, to string, err error) {
	if to, err = resolveCommit(dir, hash); err != nil {
		return "", "", err
	}
	out, err := Output(dir, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("the checkout has no commit yet")
	}
	return strings.TrimSpace(out), to, nil
}

func spanFiles(dir, from, to string) ([]ChangedFile, error) {
	if from == "" {
		return nameStatus(dir, "diff-tree", "-r", "--root", "--no-commit-id", "--no-renames", "--name-status", "-z", to)
	}
	return nameStatus(dir, "diff-tree", "-r", "--no-renames", "--name-status", "-z", from, to)
}

func spanVersions(dir, from, to, path string) (Versions, error) {
	files, err := spanFiles(dir, from, to)
	if err != nil {
		return Versions{}, err
	}
	f, ok := find(files, path)
	if !ok {
		return Versions{}, fmt.Errorf("%q is not changed in %s", path, to)
	}
	return compare(dir, path, side{spec: from + ":" + path, absent: f.Status == "A"}, side{spec: to + ":" + path, absent: f.Status == "D"}), nil
}

// CommitFiles lists the files a commit changed.
func CommitFiles(dir, hash string) ([]ChangedFile, error) {
	from, to, err := commitSpan(dir, hash)
	if err != nil {
		return nil, err
	}
	return spanFiles(dir, from, to)
}

// CommitVersions reads a file before and after a commit.
func CommitVersions(dir, hash, path string) (Versions, error) {
	from, to, err := commitSpan(dir, hash)
	if err != nil {
		return Versions{}, err
	}
	return spanVersions(dir, from, to, path)
}

// HeadFiles lists the files that differ between HEAD and a commit.
func HeadFiles(dir, hash string) ([]ChangedFile, error) {
	from, to, err := headSpan(dir, hash)
	if err != nil {
		return nil, err
	}
	return spanFiles(dir, from, to)
}

// HeadVersions reads a file at HEAD and at a commit.
func HeadVersions(dir, hash, path string) (Versions, error) {
	from, to, err := headSpan(dir, hash)
	if err != nil {
		return Versions{}, err
	}
	return spanVersions(dir, from, to, path)
}
