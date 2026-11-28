package php

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLockPHPConstraints_readsEveryPackageOnce(t *testing.T) {
	dir := t.TempDir()
	lock := `{"packages":[{"require":{"php":">=8.4.1"}},{"require":{"php":">=8.4.1"}},{"require":{"psr/log":"^3"}}],"packages-dev":[{"require":{"php":"^8.2"}}]}`
	if err := os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LockPHPConstraints(dir); !slices.Equal(got, []string{">=8.4.1", "^8.2"}) {
		t.Errorf("got %v", got)
	}
	if got := LockPHPConstraints(t.TempDir()); got != nil {
		t.Errorf("no lock: got %v", got)
	}
}

// Excluding one patch leaves the rest of its minor version, which is all an
// installed major.minor can be judged on.
func TestSatisfies_aPatchExclusionKeepsItsMinor(t *testing.T) {
	for _, c := range []string{">=8.3 <8.4 !=8.3.0", "^8.2 !=8.3.1"} {
		if !Satisfies("8.3", c) {
			t.Errorf("Satisfies(8.3, %q) = false, want true", c)
		}
	}
	if Satisfies("8.3", "!=8.3") {
		t.Error(`Satisfies(8.3, "!=8.3") = true, want false`)
	}
}
