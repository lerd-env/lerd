package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A drop asked to take the pair removes both halves, and only when the pair is
// actually there to take: the testing half has no pair of its own.
func TestDropTargets(t *testing.T) {
	cases := []struct {
		name        string
		database    string
		withTesting bool
		want        []string
	}{
		{"alone", "havenly", false, []string{"havenly"}},
		{"pair", "havenly", true, []string{"havenly", "havenly_testing"}},
		{"testing half", "havenly_testing", true, []string{"havenly_testing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dropTargets(tc.database, tc.withTesting)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("dropTargets(%q, %v) = %v, want %v", tc.database, tc.withTesting, got, tc.want)
			}
		})
	}
}

// The suffix can push a legal name past the length limit, and the sibling is
// checked before either half is touched so that never costs the parent.
func TestDatabaseDropRejectsAnUnusableSibling(t *testing.T) {
	database := strings.Repeat("a", 60)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/postgres/drop",
		strings.NewReader(`{"name":"`+database+`","testing":true}`))
	handleDatabaseDrop(rec, req, "postgres")

	got := decodeDBAction(t, rec)
	if got.OK {
		t.Fatal("handler accepted a sibling name it cannot use")
	}
	if !strings.Contains(got.Error, "longer than") {
		t.Errorf("error = %q, want a rejection of the over-long sibling", got.Error)
	}
}

// Snapshots stay unless the drop asks for them, and an older client that never
// sends the key keeps them too.
func TestDecodeDropBodyReadsTheSnapshotsChoice(t *testing.T) {
	for body, want := range map[string]bool{`{"name":"shop"}`: false, `{"name":"shop","snapshots":true}`: true} {
		req := httptest.NewRequest(http.MethodPost, "/api/databases/postgres/drop", strings.NewReader(body))
		got, ok := decodeDropBody(req)
		if !ok || got.Name != "shop" || got.Snapshots != want {
			t.Errorf("%s: got %+v, want snapshots=%v", body, got, want)
		}
	}
}
