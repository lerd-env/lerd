package shims

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestScript(t *testing.T) {
	got := script("/home/u/.local/bin/lerd", "mysqldump")
	for _, want := range []string{"#!/bin/sh", marker, "client-exec mysqldump", "\"$@\""} {
		if !strings.Contains(got, want) {
			t.Errorf("script missing %q:\n%s", want, got)
		}
	}
}

// A client-tool name comes from semi-trusted store YAML and is used as both a
// shim filename and a token in the generated script, so a name with a path
// separator, "..", or a shell/whitespace character must be rejected. Without the
// guard a name like "../../bin/lerd" writes outside BinDir and a name with a
// newline or ";" injects a line into the shim script.
func TestValidShimName(t *testing.T) {
	for _, ok := range []string{"mysqldump", "pg_dump", "mariadb-dump", "redis-cli", "psql", "mongosh"} {
		if !validShimName(ok) {
			t.Errorf("%q should be a valid shim name", ok)
		}
	}
	for _, bad := range []string{
		"", ".", "..", "../evil", "../../bin/lerd", "a/b", "a\\b",
		"foo;rm -rf /", "foo bar", "foo\nbar", "foo$(id)", "-rf", ".hidden", "a|b", "a`b`",
	} {
		if validShimName(bad) {
			t.Errorf("%q must be rejected as a shim name", bad)
		}
	}
	// A rejected name with a path separator never resolves inside BinDir, and one
	// with a newline never survives as a single-line shim: the two concrete risks.
	if p := filepath.Join("/bindir", "../../bin/lerd"); filepath.Dir(p) == "/bindir" {
		t.Errorf("path-separator name should escape BinDir under join, got %q", p)
	}
}

func TestIsShimFile(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "mysqldump")
	_ = os.WriteFile(shim, []byte(script("lerd", "mysqldump")), 0755)
	other := filepath.Join(dir, "realtool")
	_ = os.WriteFile(other, []byte("#!/bin/sh\necho hi\n"), 0755)
	if !isShimFile(shim) {
		t.Error("marked shim not detected")
	}
	if isShimFile(other) {
		t.Error("user binary wrongly detected as shim")
	}
}

func TestRemoveIfShim(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "psql")
	user := filepath.Join(dir, "psql-user")
	_ = os.WriteFile(shim, []byte(script("lerd", "psql")), 0755)
	_ = os.WriteFile(user, []byte("#!/bin/sh\n"), 0755)

	removeIfShim(shim)
	removeIfShim(user)

	if _, err := os.Stat(shim); !os.IsNotExist(err) {
		t.Error("shim should have been removed")
	}
	if _, err := os.Stat(user); err != nil {
		t.Error("user binary must never be removed")
	}
}

// The installer-owned shim names (php, composer, node…) are reserved: a store
// preset can't have the client reconcile generate over them, so it can never
// repoint the host php or composer at a database client container.
func TestReservedShimNames(t *testing.T) {
	for _, name := range []string{"php", "composer", "composer.phar", "laravel", "node", "npm", "npx", "fnm"} {
		if !isReservedShimName(name) {
			t.Errorf("%q should be reserved for the installer", name)
		}
	}
	for _, name := range []string{"mysqldump", "psql", "pg_dump", "redis-cli", "mongosh"} {
		if isReservedShimName(name) {
			t.Errorf("%q is a real client tool and must not be reserved", name)
		}
	}
}

// canWriteShim guards the reconcile's write side the way removeIfShim guards its
// remove side: an absent path or an existing lerd client shim is writable, but a
// non-shim file (an installer shim, or a user binary of the same name) is not, so
// the reconcile can never clobber it.
func TestCanWriteShim(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "mysqldump")
	if !canWriteShim(absent) {
		t.Error("an absent path must be writable")
	}

	ours := filepath.Join(dir, "psql")
	_ = os.WriteFile(ours, []byte(script("lerd", "psql")), 0755)
	if !canWriteShim(ours) {
		t.Error("our own client shim must be overwritable")
	}

	installer := filepath.Join(dir, "php")
	_ = os.WriteFile(installer, []byte("#!/bin/sh\nexec lerd php \"$@\"\n"), 0755)
	if canWriteShim(installer) {
		t.Error("an installer shim (no marker) must never be overwritten")
	}
}

func TestPruneOrphans(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	binDir := config.BinDir()
	_ = os.MkdirAll(binDir, 0755)
	live := filepath.Join(binDir, "mysqldump")
	orphan := filepath.Join(binDir, "pg_dump")
	user := filepath.Join(binDir, "sqlite3")
	_ = os.WriteFile(live, []byte(script("lerd", "mysqldump")), 0755)
	_ = os.WriteFile(orphan, []byte(script("lerd", "pg_dump")), 0755)
	_ = os.WriteFile(user, []byte("#!/bin/sh\n"), 0755)
	_ = setDecision("pg_dump", true)

	pruneOrphans(map[string]Target{"mysqldump": {Service: "mysql", Binaries: []string{"mysqldump"}}})

	if _, err := os.Stat(live); err != nil {
		t.Error("live shim must be kept")
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("orphan shim must be pruned")
	}
	if _, err := os.Stat(user); err != nil {
		t.Error("non-lerd binary must never be pruned")
	}
	if _, decided := decision("pg_dump"); decided {
		t.Error("pruned tool's decision must be forgotten")
	}
}

func TestHostHasTool(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	hostDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(hostDir, "mysqldump"), []byte("#!/bin/sh\n"), 0755)
	binDir := config.BinDir()
	_ = os.MkdirAll(binDir, 0755)
	_ = os.WriteFile(filepath.Join(binDir, "pg_dump"), []byte("#!/bin/sh\n"), 0755)
	t.Setenv("PATH", hostDir+string(os.PathListSeparator)+binDir)

	if !hostHasTool("mysqldump") {
		t.Error("host tool on PATH should be detected")
	}
	if hostHasTool("pg_dump") {
		t.Error("a tool only in lerd's bin dir must not count as host-installed")
	}
	if hostHasTool("nonexistent-tool") {
		t.Error("absent tool must not be detected")
	}
}

func TestDecisionTriState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, decided := decision("mysqldump"); decided {
		t.Fatal("fresh tool should be undecided")
	}
	_ = setDecision("mysqldump", true)
	if e, d := decision("mysqldump"); !d || !e {
		t.Fatalf("want decided+enabled, got decided=%v enabled=%v", d, e)
	}
	_ = setDecision("mysqldump", false)
	if e, d := decision("mysqldump"); !d || e {
		t.Fatalf("want decided+disabled, got decided=%v enabled=%v", d, e)
	}
	_ = forgetDecision("mysqldump")
	if _, decided := decision("mysqldump"); decided {
		t.Fatal("forgotten tool should be undecided again")
	}
}

func TestDecideAutoEnablesWhenHostLacksTool(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // host has nothing

	enabled, decided := decide("mysqldump", nil)
	if !enabled || !decided {
		t.Fatalf("host lacking the tool should auto-enable, got enabled=%v decided=%v", enabled, decided)
	}
}

func TestDecideLeavesConflictUndecidedWithoutPrompter(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	hostDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(hostDir, "mysqldump"), []byte("#!/bin/sh\n"), 0755)
	t.Setenv("PATH", hostDir)

	enabled, decided := decide("mysqldump", nil)
	if enabled || decided {
		t.Fatalf("a host conflict with no prompter must stay undecided, got enabled=%v decided=%v", enabled, decided)
	}
}
