package config

import "testing"

func TestSeatReleasedMarker(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if SeatReleased() {
		t.Fatal("a fresh install reads as released")
	}
	if err := MarkSeatReleased(); err != nil {
		t.Fatal(err)
	}
	if !SeatReleased() {
		t.Fatal("the marker is not read back")
	}
	if err := ClearSeatReleased(); err != nil {
		t.Fatal(err)
	}
	if SeatReleased() {
		t.Fatal("the marker survives a clear")
	}
	if err := ClearSeatReleased(); err != nil {
		t.Fatalf("clearing a missing marker errors: %v", err)
	}
}
