package cleanup

import "testing"

func TestParseHumanSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"0B", 0, true},
		{"11.87kB", 11870, true},
		{"8.698MB", 8698000, true},
		{"1.748GB", 1748000000, true},
		{"553.5MB", 553500000, true},
		{"", 0, false},
		{"MB", 0, false},
		{"12", 0, false},
		{"12ZB", 0, false},
	}
	for _, c := range cases {
		got, ok := parseHumanSize(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseHumanSize(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The image rows carry a variable-width CREATED phrase, so the parser has to
// read the id from the left and the sizes from the right.
func TestParseUniqueBytes(t *testing.T) {
	out := `Images space usage:

REPOSITORY                TAG            IMAGE ID      CREATED          SIZE        SHARED SIZE  UNIQUE SIZE  CONTAINERS
docker.io/library/redis   7-alpine       2a51817f7925  7 weeks          39.9MB      0B           39.9MB       1
docker.io/library/alpine  latest         d529dd0c6e55  3 months         8.71MB      8.698MB      11.87kB      0
localhost/lerd-php85-fpm  local          cc0267b57734  About a minute   553.5MB     553.5MB      24.88kB      1
<none>                    <none>         23c2136424f4  5 days           553.3MB     553.3MB      22.69kB      0

Containers space usage:

CONTAINER ID  IMAGE         COMMAND  LOCAL VOLUMES  SIZE     CREATED    STATUS   NAMES
e0b54fc69136  f25c74398063           2              11.61kB  5 minutes  running  lerd-mailpit
`
	got := parseUniqueBytes(out)
	want := map[string]int64{
		"2a51817f7925": 39900000,
		"d529dd0c6e55": 11870,
		"cc0267b57734": 24880,
		"23c2136424f4": 22690,
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d rows, want %d: %v", len(got), len(want), got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("unique[%s] = %d, want %d", id, got[id], w)
		}
	}
}

func TestParseUniqueBytes_NoImageSection(t *testing.T) {
	if got := parseUniqueBytes("Error: cannot connect to podman\n"); len(got) != 0 {
		t.Fatalf("want no rows, got %v", got)
	}
}

func TestParseStoreBytes(t *testing.T) {
	out := `[
    {"Type":"Images","Total":27,"Active":11,"RawSize":3295408786,"RawReclaimable":1748284535},
    {"Type":"Containers","Total":9,"Active":9,"RawSize":116553,"RawReclaimable":0}
]`
	if got := parseStoreBytes(out); got != 3295408786 {
		t.Fatalf("parseStoreBytes = %d, want 3295408786", got)
	}
	if got := parseStoreBytes("not json"); got != 0 {
		t.Fatalf("parseStoreBytes(garbage) = %d, want 0", got)
	}
}

// podman images reports SharedSize as zero, so the enrichment is what stops a
// shared base being charged to every image built on it.
func TestApplyUniqueBytes(t *testing.T) {
	imgs := []image{
		{ID: "sha256:cc0267b57734aaaabbbbccccddddeeeeffff0000111122223333444455556666", Size: 553_500_000},
		{ID: "sha256:d529dd0c6e55aaaabbbbccccddddeeeeffff0000111122223333444455556666", Size: 8_710_000},
		{ID: "sha256:9999999999990000aaaabbbbccccddddeeeeffff11112222333344445555aaaa", Size: 42},
	}
	applyUniqueBytes(imgs, map[string]int64{
		"cc0267b57734": 24_880,
		"d529dd0c6e55": 11_870,
	})
	if got := reclaimable(imgs[0]); got != 24_880 {
		t.Errorf("shared base image reclaimable = %d, want 24880", got)
	}
	if got := reclaimable(imgs[1]); got != 11_870 {
		t.Errorf("alpine reclaimable = %d, want 11870", got)
	}
	// An image podman df did not list keeps whatever podman images reported,
	// so a df that cannot answer degrades to the old behaviour rather than
	// reporting nothing reclaimable.
	if got := reclaimable(imgs[2]); got != 42 {
		t.Errorf("unlisted image reclaimable = %d, want 42", got)
	}
}

func TestUsedTotal_SubtractsOnlyForeignBytes(t *testing.T) {
	lerd := image{ID: "aaaaaaaaaaaa", Size: 500, SharedSize: 400, Labels: map[string]string{"dev.lerd.fpm.base-digest": "x"}}
	foreign := image{ID: "bbbbbbbbbbbb", Names: []string{"docker.io/library/postgres:17"}, Size: 300, SharedSize: 100}
	prev := readStoreBytes
	readStoreBytes = func() int64 { return 1000 }
	t.Cleanup(func() { readStoreBytes = prev })

	// The store is 1000 and postgres alone holds 200 of it, so lerd's share is
	// the remaining 800 with its own shared layers counted once.
	if got := usedTotal([]image{lerd, foreign}, map[string]bool{}, map[string]bool{}); got != 800 {
		t.Fatalf("usedTotal = %d, want 800", got)
	}
}

func TestUsedTotal_ZeroWhenPodmanCannotAnswer(t *testing.T) {
	prev := readStoreBytes
	readStoreBytes = func() int64 { return 0 }
	t.Cleanup(func() { readStoreBytes = prev })
	if got := usedTotal(nil, map[string]bool{}, map[string]bool{}); got != 0 {
		t.Fatalf("usedTotal = %d, want 0", got)
	}
}

// Two targets sharing a layer free more than either is credited with, so Apply
// reports the measured store delta rather than the sum of its estimates.
func TestApply_ReportsMeasuredStoreDelta(t *testing.T) {
	withImages(t, nil, nil)
	store := int64(1000)
	prevStore, prevRemove := readStoreBytes, removeImage
	readStoreBytes = func() int64 { return store }
	removeImage = func(string) error { store -= 300; return nil }
	t.Cleanup(func() { readStoreBytes, removeImage = prevStore, prevRemove })

	plan := Plan{Targets: []Target{
		{Kind: KindImage, ID: "a", Bytes: 10},
		{Kind: KindImage, ID: "b", Bytes: 10},
	}}
	removed, reclaimed := Apply(plan)
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if reclaimed != 600 {
		t.Fatalf("reclaimed = %d, want the measured 600", reclaimed)
	}
}

func TestApply_FallsBackToEstimateWithoutAStoreReading(t *testing.T) {
	withImages(t, nil, nil)
	prevStore, prevRemove := readStoreBytes, removeImage
	readStoreBytes = func() int64 { return 0 }
	removeImage = func(string) error { return nil }
	t.Cleanup(func() { readStoreBytes, removeImage = prevStore, prevRemove })

	_, reclaimed := Apply(Plan{Targets: []Target{{Kind: KindImage, ID: "a", Bytes: 25}}})
	if reclaimed != 25 {
		t.Fatalf("reclaimed = %d, want the 25 estimate", reclaimed)
	}
}
