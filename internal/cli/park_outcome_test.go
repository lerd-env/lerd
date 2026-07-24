package cli

import "testing"

func TestParkTally(t *testing.T) {
	cases := []struct {
		linked, skipped, failed int
		want                    string
	}{
		{50, 0, 0, "50 linked"},
		{48, 2, 0, "48 linked, 2 skipped"},
		{47, 2, 1, "47 linked, 2 skipped, 1 failed"},
		{0, 3, 0, "0 linked, 3 skipped"},
	}
	for _, c := range cases {
		if got := parkTally(c.linked, c.skipped, c.failed); got != c.want {
			t.Errorf("parkTally(%d, %d, %d) = %q, want %q", c.linked, c.skipped, c.failed, got, c.want)
		}
	}
}

// "no PHP projects found" is only true when nothing in the tree looked like a
// project at all; a directory of already-registered sites must not say it.
func TestParkAllUnrecognised(t *testing.T) {
	cases := []struct {
		name    string
		skipped []ParkOutcome
		want    bool
	}{
		{"nothing skipped", nil, true},
		{
			"all unrecognised",
			[]ParkOutcome{{Reason: reasonNotPHP}, {Reason: reasonNotPHP}},
			true,
		},
		{
			"one already registered",
			[]ParkOutcome{{Reason: reasonNotPHP}, {Reason: "already registered as blog"}},
			false,
		},
		{
			"one declares its own runtime",
			[]ParkOutcome{{Reason: "declares its own host-proxy runtime; run 'lerd link' in it"}},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parkAllUnrecognised(c.skipped); got != c.want {
				t.Errorf("parkAllUnrecognised = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParkAdmits(t *testing.T) {
	t.Run("a directory with no PHP in it is not a project", func(t *testing.T) {
		if parkAdmits(t.TempDir()) {
			t.Error("an empty directory must not be admitted")
		}
	})

	t.Run("a composer.json makes it a project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "composer.json", `{"name":"acme/app"}`)
		if !parkAdmits(dir) {
			t.Error("a directory with composer.json must be admitted")
		}
	})

	t.Run("a top-level PHP file makes it a project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "index.php", "<?php echo 1;")
		if !parkAdmits(dir) {
			t.Error("a directory with a top-level PHP file must be admitted")
		}
	})
}
