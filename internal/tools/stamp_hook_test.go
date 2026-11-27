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

// Reading mise's version rewrites its stamp on every status build. Announcing
// that as an install made lerd-ui rebuild its status, which read the version
// again, about once a second for as long as a dashboard was open.
func TestWriteStampStaysQuietWhenTheVersionIsUnchanged(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := OnStampWritten
	t.Cleanup(func() { OnStampWritten = prev })
	called := 0
	OnStampWritten = func() { called++ }

	for _, v := range []string{"2026.11.14", "2026.11.14", "2026.11.15"} {
		if err := WriteStamp("mise", v); err != nil {
			t.Fatalf("WriteStamp(%s): %v", v, err)
		}
	}
	if called != 2 {
		t.Errorf("hook ran %d times, want 2: once per version, not per write", called)
	}
}
