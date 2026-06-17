//go:build linux

package logsource

import (
	"slices"
	"strconv"
	"testing"
)

// nFlag returns the value following "-n" in an argv, or "".
func nFlag(args []string) string {
	for i, a := range args {
		if a == "-n" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestJournalArgs_ResumeCapsAtMaxLinesNotTheSmallTail(t *testing.T) {
	src := Source{Name: "ui", Kind: KindJournal, Locator: "lerd-ui"}
	cursor := "s=abc;i=1f;b=2;m=3;t=4;x=5"

	resume, _ := journalArgs(src, Opts{Since: cursor, Lines: 50})
	if !slices.Contains(resume, "--after-cursor="+cursor) {
		t.Errorf("resume args must carry --after-cursor, got %v", resume)
	}
	// Resume must stay bounded (memory) but not clamp to the small per-call tail
	// (which would drop a busy unit's gap lines): it caps at maxLines.
	if got := nFlag(resume); got != strconv.Itoa(maxLines) {
		t.Errorf("resume -n = %q, want maxLines %d so gap lines aren't dropped to the 50-line tail", got, maxLines)
	}

	fresh, _ := journalArgs(src, Opts{Lines: 50})
	if got := nFlag(fresh); got != "50" {
		t.Errorf("a fresh read should tail with -n 50, got %q", got)
	}
}

func TestJournalTime_NegativeDurationIsLookBack(t *testing.T) {
	if got := journalTime("15m"); got != "-15m" {
		t.Errorf("journalTime(15m) = %q, want -15m", got)
	}
	if got := journalTime("-15m"); got != "-15m" {
		t.Errorf("journalTime(-15m) = %q, want -15m (not the invalid --15m)", got)
	}
}
