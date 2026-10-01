//go:build windows

package cli

import (
	"slices"
	"testing"
)

func TestHostMemoryGiBIsPlausible(t *testing.T) {
	if g := hostMemoryGiB(); g < 1 || g > 8192 {
		t.Errorf("hostMemoryGiB() = %d", g)
	}
}

func TestMachineInitArgsRootfulWithMemory(t *testing.T) {
	got := machineInitArgs("", 6144)
	want := []string{"machine", "init", "--rootful", "--memory", "6144"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := machineInitArgs("dev", 0); !slices.Equal(got, []string{"machine", "init", "--rootful", "dev"}) {
		t.Errorf("named, no memory: %v", got)
	}
}
