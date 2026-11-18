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

var fakeWrite WriteFunc

// fakeRoot stands in for sudo: writes land in a temp dir, and pfctl fails
// when loadErr is set.
func fakeRoot(t *testing.T, loadErr error) *[]string {
	t.Helper()
	useAnchor(t, "")
	prevGrant, prevSudo := SudoersPath, runSudo
	SudoersPath = filepath.Join(t.TempDir(), "lerd-pf")
	var calls []string
	fakeWrite = func(path string, content []byte, mode os.FileMode) error {
		calls = append(calls, "write "+path)
		return os.WriteFile(path, content, mode)
	}
	runSudo = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if slices.Contains(args, pfctl) {
			return loadErr
		}
		if args[0] == "rm" {
			for _, p := range args[2:] {
				os.Remove(p)
			}
		}
		return nil
	}
	t.Cleanup(func() { SudoersPath, runSudo = prevGrant, prevSudo })
	return &calls
}

func TestInstallWritesGrantBeforeAnchorAndLoadsIt(t *testing.T) {
	calls := fakeRoot(t, nil)
	moved, err := Install("alice", fakeWrite)
	if err != nil || !moved {
		t.Fatalf("Install() = %v, %v, want moved and no error", moved, err)
	}
	if len(*calls) != 3 || (*calls)[0] != "write "+SudoersPath || (*calls)[1] != "write "+AnchorPath {
		t.Errorf("calls = %q, want the grant, then the anchor, then the load", *calls)
	}
	if len(Redirected()) != 2 {
		t.Error("the anchor should be in place after a good load")
	}
}

// An anchor pf would not load must not stay behind, or nginx moves to a port
// nothing redirects to and every site goes down.
func TestInstallTakesTheAnchorBackWhenPfWillNotLoad(t *testing.T) {
	fakeRoot(t, errors.New("pfctl: Operation not permitted"))
	moved, err := Install("alice", fakeWrite)
	if err == nil {
		t.Fatal("a failed load must be reported")
	}
	if moved {
		t.Error("nginx never moved, so it needs no restart")
	}
	if len(Redirected()) != 0 {
		t.Error("the anchor stayed behind after pf refused it")
	}
}

func TestInstallMovesNginxBackWhenAnExistingAnchorWillNotLoad(t *testing.T) {
	fakeRoot(t, nil)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	runSudo = func(args ...string) error {
		if args[0] == "rm" {
			os.Remove(AnchorPath)
			return nil
		}
		return errors.New("pfctl: refused")
	}
	moved, err := Install("alice", fakeWrite)
	if err == nil || !moved || len(Redirected()) != 0 {
		t.Errorf("Install() = %v, %v with anchor %v, want nginx moved back to its wide bind", moved, err, Redirected())
	}
}
