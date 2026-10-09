package applog

import (
	"fmt"
	"testing"
)

func entriesN(n int) []LogEntry {
	out := make([]LogEntry, n)
	for i := range out {
		out[i] = LogEntry{Message: fmt.Sprint(i)}
	}
	return out
}

func TestPage_SlicesFromTheOffset(t *testing.T) {
	got, more := Page(entriesN(5), 1, 2)
	if len(got) != 2 || got[0].Message != "1" || got[1].Message != "2" {
		t.Fatalf("page = %v, want entries 1 and 2", got)
	}
	if !more {
		t.Error("more = false with entries left after the page")
	}
}

func TestPage_LastPageSaysNoMore(t *testing.T) {
	got, more := Page(entriesN(5), 3, 2)
	if len(got) != 2 || more {
		t.Fatalf("page = %v more = %v, want the last two and no more", got, more)
	}
}

func TestPage_PastTheEndIsEmpty(t *testing.T) {
	got, more := Page(entriesN(5), 9, 2)
	if len(got) != 0 || more {
		t.Fatalf("page = %v more = %v, want nothing", got, more)
	}
}

func TestMarkPastTail_FlagsOnlyEntriesOlderThanTheTail(t *testing.T) {
	page, _ := Page(entriesN(10), 4, 4)
	MarkPastTail(page, 4, 6)
	for i, e := range page {
		if want := 4+i >= 6; e.PastTail != want {
			t.Errorf("entry %d PastTail = %v, want %v", 4+i, e.PastTail, want)
		}
	}
}
