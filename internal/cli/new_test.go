package cli

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The "Next" hint must cd into the path the user actually typed. Using only the
// base name breaks for nested targets (lerd new apps/myapp → cd myapp).
func TestNewNextStep(t *testing.T) {
	cases := []struct {
		target  string
		chained bool
		want    string
	}{
		{"myapp", false, "cd myapp && lerd link && lerd setup"},
		{"apps/myapp", false, "cd apps/myapp && lerd link && lerd setup"},
		{"/abs/path/myapp", false, "cd /abs/path/myapp && lerd link && lerd setup"},
		// A run that linked and set the project up itself has nothing left to
		// suggest but the move it cannot make on the user's behalf.
		{"myapp", true, "cd myapp"},
		{"apps/myapp", true, "cd apps/myapp"},
	}
	for _, tc := range cases {
		if got := newNextStep(tc.target, tc.chained); got != tc.want {
			t.Errorf("newNextStep(%q, %v) = %q, want %q", tc.target, tc.chained, got, tc.want)
		}
	}
}

// `lerd new` with no name at all has to reach the wizard rather than index a
// positional that isn't there, and `lerd new -- --flag` names no project either:
// both arrive as a bare list, so only the dash position tells them apart.
func TestNewArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		dash       int
		wantTarget string
		wantExtra  []string
	}{
		{"no arguments", nil, -1, "", nil},
		{"target only", []string{"myapp"}, -1, "myapp", []string{}},
		{"target and extras", []string{"myapp", "--dev"}, 1, "myapp", []string{"--dev"}},
		{"extras with no target", []string{"--dev"}, 0, "", []string{"--dev"}},
	}
	for _, tc := range cases {
		target, extra := newArgs(tc.args, tc.dash)
		if target != tc.wantTarget {
			t.Errorf("%s: target = %q, want %q", tc.name, target, tc.wantTarget)
		}
		if len(extra) != len(tc.wantExtra) {
			t.Errorf("%s: extra = %v, want %v", tc.name, extra, tc.wantExtra)
		}
	}
}

// runNewCmd parses args through the real command, stubbing out the scaffold so
// the test only observes what flag parsing produced.
func runNewCmd(t *testing.T, args ...string) (target, framework string, extra []string, err error) {
	t.Helper()
	cmd := NewNewCmd()
	cmd.RunE = func(c *cobra.Command, positional []string) error {
		target, extra = newArgs(positional, c.ArgsLenAtDash())
		framework, _ = c.Flags().GetString("framework")
		return nil
	}
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err = cmd.Execute()
	return
}

// The help text advertises `lerd new myapp --framework=symfony`, so a flag after
// the positional must be lerd's own, not forwarded to composer.
func TestNewCmdParsesFrameworkAfterTarget(t *testing.T) {
	target, framework, extra, err := runNewCmd(t, "magento-real", "--framework=magento")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if target != "magento-real" {
		t.Errorf("target = %q, want magento-real", target)
	}
	if framework != "magento" {
		t.Errorf("framework = %q, want magento", framework)
	}
	if len(extra) != 0 {
		t.Errorf("extra args = %v, want none", extra)
	}
}

func TestNewCmdParsesFrameworkBeforeTarget(t *testing.T) {
	target, framework, _, err := runNewCmd(t, "--framework=magento", "magento-real")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if target != "magento-real" || framework != "magento" {
		t.Errorf("target = %q, framework = %q, want magento-real/magento", target, framework)
	}
}

// Everything after -- still belongs to the scaffold command, flags included.
func TestNewCmdForwardsArgsAfterDash(t *testing.T) {
	target, framework, extra, err := runNewCmd(t, "myapp", "--", "--no-interaction", "--dev")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if target != "myapp" {
		t.Errorf("target = %q, want myapp", target)
	}
	// The flag carries no default any more: an empty value is what tells the
	// command to ask, and runNew is where a non-interactive run falls back.
	if framework != "" {
		t.Errorf("framework = %q, want it unset", framework)
	}
	if want := []string{"--no-interaction", "--dev"}; !reflect.DeepEqual(extra, want) {
		t.Errorf("extra args = %v, want %v", extra, want)
	}
}

// An unknown flag before -- is a mistake, not scaffold input: forwarding it is
// what made composer die with `The "--framework" option does not exist`.
func TestNewCmdRejectsUnknownFlagBeforeDash(t *testing.T) {
	_, _, _, err := runNewCmd(t, "myapp", "--no-interaction")
	if err == nil {
		t.Fatal("expected an error for an unconsumed flag, got none")
	}
	if !strings.Contains(err.Error(), "--no-interaction") {
		t.Errorf("error = %v, want it to name the offending flag", err)
	}
}

// stubMountSeams swaps the podman lookups for fixed answers and records the
// mount call, so the scaffold guard can be driven without touching quadlets.
func stubMountSeams(t *testing.T, autoMountable, visible bool) *string {
	t.Helper()
	mounted := new(string)
	origMount, origAuto, origVisible := ensurePathMounted, pathAutoMountable, pathVisible
	origSees := containerSeesHostDir
	ensurePathMounted = func(path, phpVersion string) { *mounted = path }
	pathAutoMountable = func(path string) bool { return autoMountable }
	pathVisible = func(path, phpVersion string) bool { return visible }
	containerSeesHostDir = func(path, phpVersion string) bool { return true }
	t.Cleanup(func() {
		ensurePathMounted, pathAutoMountable, pathVisible = origMount, origAuto, origVisible
		containerSeesHostDir = origSees
	})
	return mounted
}

// The scaffold runs composer through the container shim, so the target's parent
// has to exist on the host and be bind-mounted before the container starts.
func TestPrepareScaffoldParentMountsParent(t *testing.T) {
	mounted := stubMountSeams(t, true, false)

	parent := filepath.Join(t.TempDir(), "out", "of", "home")
	if err := prepareScaffoldParent(filepath.Join(parent, "oobtest")); err != nil {
		t.Fatalf("prepareScaffoldParent: %v", err)
	}
	if *mounted != parent {
		t.Errorf("mounted %q, want %q", *mounted, parent)
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		t.Errorf("parent %q was not created: %v", parent, err)
	}
}

// A parent lerd will never bind-mount (a temp dir) and that no parked volume
// covers must fail up front, not as a crun exit 127 out of composer.
func TestPrepareScaffoldParentRejectsUnmountableParent(t *testing.T) {
	mounted := stubMountSeams(t, false, false)

	err := prepareScaffoldParent(filepath.Join(t.TempDir(), "oob", "app"))
	if err == nil {
		t.Fatal("expected an error for a parent lerd cannot mount")
	}
	if !strings.Contains(err.Error(), "lerd park") {
		t.Errorf("error = %v, want it to suggest parking the parent", err)
	}
	if *mounted != "" {
		t.Errorf("mounted %q, want no mount attempt", *mounted)
	}
}

// Parking a temp dir is how you opt in, so an already-visible parent scaffolds.
func TestPrepareScaffoldParentAcceptsVisibleParent(t *testing.T) {
	stubMountSeams(t, false, true)

	if err := prepareScaffoldParent(filepath.Join(t.TempDir(), "parked", "app")); err != nil {
		t.Fatalf("prepareScaffoldParent: %v", err)
	}
}

// A parent we cannot create must fail before composer runs, with a message that
// points at the directory rather than a crun exit 127.
func TestPrepareScaffoldParentFailsOnUncreatableParent(t *testing.T) {
	mounted := stubMountSeams(t, true, false)

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	err := prepareScaffoldParent(filepath.Join(blocker, "sub", "app"))
	if err == nil {
		t.Fatal("expected an error when the parent cannot be created")
	}
	if !strings.Contains(err.Error(), filepath.Join(blocker, "sub")) {
		t.Errorf("error = %v, want it to name the parent directory", err)
	}
	if *mounted != "" {
		t.Error("must not try to mount a parent that does not exist")
	}
}
