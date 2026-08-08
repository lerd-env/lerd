//go:build linux

package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCgroupStatKey(t *testing.T) {
	dir := t.TempDir()
	stat := filepath.Join(dir, "memory.stat")
	body := "anon 24117248\nfile 664797184\ninactive_file 600000000\nkernel 38797312\n"
	if err := os.WriteFile(stat, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readCgroupStatKey(stat, "inactive_file"); got != 600000000 {
		t.Errorf("inactive_file = %d, want 600000000", got)
	}
	if got := readCgroupStatKey(stat, "anon"); got != 24117248 {
		t.Errorf("anon = %d, want 24117248", got)
	}
	if got := readCgroupStatKey(stat, "missing"); got != 0 {
		t.Errorf("missing key = %d, want 0", got)
	}
	if got := readCgroupStatKey(filepath.Join(dir, "nope"), "anon"); got != 0 {
		t.Errorf("unreadable file = %d, want 0", got)
	}
}

// fakeCgroup writes a memory.current/memory.stat pair into a fixture tree and
// points cgroupRoot at it, returning the unit's cgroup path.
func fakeCgroup(t *testing.T, current, stat string) string {
	t.Helper()
	root := t.TempDir()
	cg := "/user.slice/user@1000.service/app.slice/lerd-ui.service"
	dir := filepath.Join(root, cg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.current"), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	if stat != "" {
		if err := os.WriteFile(filepath.Join(dir, "memory.stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev := cgroupRoot
	cgroupRoot = root
	t.Cleanup(func() { cgroupRoot = prev })
	return cg
}

// The numbers are a real lerd-ui after three hours: 2 GB of memory.current, all
// of it page cache the poller read twice so none of it is on the inactive list.
// Subtracting only inactive_file leaves the whole 2 GB in the row.
func TestCgroupMemoryHeld_ExcludesActiveCacheToo(t *testing.T) {
	cg := fakeCgroup(t, "2048086016\n",
		"anon 24117248\nfile 1942446080\nkernel 73400320\nshmem 0\ninactive_file 0\nactive_file 1942446080\n")

	held, ok := cgroupMemoryHeld(cg)
	if !ok {
		t.Fatal("cgroupMemoryHeld reported no data for a populated cgroup")
	}
	if want := int64(105639936); held != want {
		t.Errorf("held = %d, want %d (current - file, not current - inactive_file)", held, want)
	}
}

// Shared memory is counted inside the file total but the kernel cannot drop it,
// so it has to stay in the number a service is reported holding.
func TestCgroupMemoryHeld_KeepsSharedMemory(t *testing.T) {
	cg := fakeCgroup(t, "1000000000\n",
		"anon 600000000\nfile 380000000\nshmem 80000000\nkernel 20000000\ninactive_file 300000000\n")

	held, ok := cgroupMemoryHeld(cg)
	if !ok {
		t.Fatal("cgroupMemoryHeld reported no data for a populated cgroup")
	}
	if want := int64(700000000); held != want {
		t.Errorf("held = %d, want %d (shmem stays counted)", held, want)
	}
}

// memory.current and memory.stat are read separately and can disagree under a
// racing reclaim; never report a negative row.
func TestCgroupMemoryHeld_FallsBackWhenStatOutrunsCurrent(t *testing.T) {
	cg := fakeCgroup(t, "100000\n", "file 900000\nshmem 0\n")

	held, ok := cgroupMemoryHeld(cg)
	if !ok || held != 100000 {
		t.Errorf("held = %d ok = %v, want 100000 true (fall back to memory.current)", held, ok)
	}
}

func TestCgroupMemoryHeld_NoData(t *testing.T) {
	if _, ok := cgroupMemoryHeld(""); ok {
		t.Error("empty cgroup path should report ok=false so the caller keeps MemoryCurrent")
	}
	root := t.TempDir()
	prev := cgroupRoot
	cgroupRoot = root
	t.Cleanup(func() { cgroupRoot = prev })
	if _, ok := cgroupMemoryHeld("/user.slice/gone.service"); ok {
		t.Error("missing memory.current should report ok=false")
	}
}
