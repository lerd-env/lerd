package screenshare

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseActive(t *testing.T) {
	cases := map[string]bool{
		"webcam-only.json":        false,
		"webcam-in-use.json":      false,
		"kwin-share.json":         true,
		"video-source-share.json": true,
		"plasma-thumbnail.json":   false,
		"unwatched-stream.json":   false,
	}
	for name, want := range cases {
		got, err := ParseActive(fixture(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s: ParseActive = %v, want %v", name, got, want)
		}
	}
}

func TestParseActiveRejectsGarbage(t *testing.T) {
	if _, err := ParseActive([]byte("not json")); err == nil {
		t.Error("want an error for output that is not a pw-dump array")
	}
}

// pw-dump -m prints the full graph, then one array per change, and a removed
// object comes back with a null info.
func TestFollowReportsEachFlip(t *testing.T) {
	stream := string(fixture(t, "webcam-only.json")) +
		string(fixture(t, "kwin-share.json")) +
		`[{"id": 170, "type": "PipeWire:Interface:Link", "info": null}]` +
		`[{"id": 69, "type": "PipeWire:Interface:Node", "info": null}]`
	var got []bool
	if err := follow(strings.NewReader(stream), func(active bool) { got = append(got, active) }); err != nil {
		t.Fatal(err)
	}
	want := []bool{false, true, false}
	if len(got) != len(want) {
		t.Fatalf("flips = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flips = %v, want %v", got, want)
		}
	}
}
