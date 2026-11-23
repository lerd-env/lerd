//go:build darwin

package pfredirect

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeRoot stands in for sudo, with an anchor for 80 and 443 in place.
func fakeRoot(t *testing.T) *[]string {
	t.Helper()
	useAnchor(t, Render([]int{80, 443}))
	prevGrant, prevSudo, prevReaches := SudoersPath, runSudo, reaches
	SudoersPath = filepath.Join(t.TempDir(), "lerd-pf")
	reaches = func(int, int) bool { return true }
	var calls []string
	runSudo = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "rm" {
			for _, p := range args[2:] {
				os.Remove(p)
			}
		}
		return nil
	}
	t.Cleanup(func() { SudoersPath, runSudo, reaches = prevGrant, prevSudo, prevReaches })
	return &calls
}

func TestApplyLoadsTheAnchorWithoutPrompting(t *testing.T) {
	calls := fakeRoot(t)
	if err := Apply(); err != nil {
		t.Fatal(err)
	}
	if want := "-n " + strings.Join(ApplyArgs(), " "); len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("calls = %q, want only %q", *calls, want)
	}
}

func TestApplyReportsARedirectLoopbackBypasses(t *testing.T) {
	fakeRoot(t)
	reaches = func(from, _ int) bool { return from != 443 }
	if err := Apply(); !errors.Is(err, ErrNotEffective) {
		t.Errorf("Apply() = %v, want ErrNotEffective", err)
	}
}

func TestRemoveFlushesTheAnchorThenDeletesItAndTheGrant(t *testing.T) {
	calls := fakeRoot(t)
	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	flush := strings.Join([]string{pfctl, "-a", Anchor, "-F", "all"}, " ")
	if len(*calls) != 2 || (*calls)[0] != flush || !strings.HasPrefix((*calls)[1], "rm -f") {
		t.Errorf("calls = %q, want the flush, then the files removed", *calls)
	}
	if len(Redirected()) != 0 {
		t.Error("the anchor should be gone")
	}
	if !slices.Contains(strings.Fields((*calls)[1]), SudoersPath) {
		t.Errorf("the grant was not removed: %q", (*calls)[1])
	}
}
