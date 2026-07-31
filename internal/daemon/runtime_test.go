package daemon

import (
	"runtime"
	"testing"
)

func TestTargetProcs_capsManyCoreMachines(t *testing.T) {
	for _, tc := range []struct {
		numCPU int
		want   int
	}{
		{numCPU: 1, want: 1},
		{numCPU: 2, want: 2},
		{numCPU: maxProcs, want: maxProcs},
		{numCPU: 8, want: maxProcs},
		{numCPU: 32, want: maxProcs},
		{numCPU: 128, want: maxProcs},
	} {
		if got := targetProcs(tc.numCPU); got != tc.want {
			t.Errorf("targetProcs(%d) = %d, want %d", tc.numCPU, got, tc.want)
		}
	}
}

// A machine smaller than the cap must not be scaled up: the cap only ever
// removes parallelism the daemon has no use for.
func TestTargetProcs_neverRaisesSmallMachines(t *testing.T) {
	for n := 1; n <= maxProcs; n++ {
		if got := targetProcs(n); got != n {
			t.Errorf("targetProcs(%d) = %d, want it left alone", n, got)
		}
	}
}

func TestTargetProcs_zeroOrNegativeIsSafe(t *testing.T) {
	for _, n := range []int{0, -1} {
		if got := targetProcs(n); got < 1 {
			t.Errorf("targetProcs(%d) = %d, want at least 1", n, got)
		}
	}
}

func TestTuneRuntime_appliesTheCap(t *testing.T) {
	before := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(before) })

	got := TuneRuntime()
	if want := targetProcs(runtime.NumCPU()); got != want {
		t.Errorf("TuneRuntime() = %d, want %d", got, want)
	}
	if actual := runtime.GOMAXPROCS(0); actual != got {
		t.Errorf("GOMAXPROCS is %d, want the reported %d", actual, got)
	}
}

// An operator who set GOMAXPROCS explicitly outranks the cap.
func TestTuneRuntime_respectsExplicitOverride(t *testing.T) {
	before := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(before) })

	t.Setenv("GOMAXPROCS", "7")
	runtime.GOMAXPROCS(7)

	if got := TuneRuntime(); got != 7 {
		t.Errorf("TuneRuntime() = %d, want the operator's 7", got)
	}
}
