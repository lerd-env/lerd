package tools

import "testing"

// Every tool install, native PHP builds included, ends in WriteStamp, so that is
// where lerd-ui learns its cached update answers are out of date.
func TestWriteStampAnnouncesTheInstall(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := OnStampWritten
	t.Cleanup(func() { OnStampWritten = prev })
	called := 0
	OnStampWritten = func() { called++ }

	if err := WriteStamp("composer", "2.9.0"); err != nil {
		t.Fatalf("WriteStamp: %v", err)
	}
	if called != 1 {
		t.Errorf("hook ran %d times, want 1", called)
	}
}
