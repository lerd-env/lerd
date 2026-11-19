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
	prevGrant, prevSudo, prevReaches := SudoersPath, runSudo, reaches
	SudoersPath = filepath.Join(t.TempDir(), "lerd-pf")
	reaches = func(int, int) bool { return true }
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
	t.Cleanup(func() { SudoersPath, runSudo, reaches = prevGrant, prevSudo, prevReaches })
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

// pf can take the anchor and still never apply it, when another ruleset skips
// lo0, which leaves the sites down exactly as a refused load would.
func TestInstallTakesTheAnchorBackWhenLoopbackBypassesIt(t *testing.T) {
	fakeRoot(t, nil)
	reaches = func(int, int) bool { return false }
	moved, err := Install("alice", fakeWrite)
	if !errors.Is(err, ErrNotEffective) {
		t.Fatalf("Install() error = %v, want ErrNotEffective", err)
	}
	if moved || len(Redirected()) != 0 {
		t.Errorf("Install() moved = %v with anchor %v, want nginx left on its wide bind", moved, Redirected())
	}
}

func TestApplyReportsARedirectLoopbackBypasses(t *testing.T) {
	fakeRoot(t, nil)
	if _, err := Install("alice", fakeWrite); err != nil {
		t.Fatal(err)
	}
	reaches = func(from, _ int) bool { return from != 443 }
	if err := Apply(); !errors.Is(err, ErrNotEffective) {
		t.Errorf("Apply() = %v, want ErrNotEffective", err)
	}
}
