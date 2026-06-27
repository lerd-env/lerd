package podman

import "testing"

func TestParsePodmanVersion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		major   int
		minor   int
		wantErr bool
	}{
		{"plain", "podman version 5.8.2\n", 5, 8, false},
		{"trailing space", "podman version 4.9.3 \n", 4, 9, false},
		{"debian suffix", "podman version 4.9.3+ds1\n", 4, 9, false},
		{"5.0 boundary", "podman version 5.0.0\n", 5, 0, false},
		{"no patch", "podman version 4.9\n", 4, 9, false},
		{"garbage", "not a version line\n", 0, 0, true},
		{"empty", "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, err := parsePodmanVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got major=%d minor=%d", major, minor)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if major != tt.major || minor != tt.minor {
				t.Errorf("got %d.%d, want %d.%d", major, minor, tt.major, tt.minor)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		major, minor       int
		wantMajor, wantMin int
		want               bool
	}{
		{4, 5, 4, 5, true},   // exact match
		{4, 4, 4, 5, false},  // one minor short
		{4, 9, 4, 5, true},   // newer minor
		{5, 0, 4, 5, true},   // newer major beats lower minor
		{3, 99, 4, 5, false}, // older major
		{4, 10, 4, 5, true},  // double-digit minor
	}
	for _, tt := range tests {
		got := versionAtLeast(tt.major, tt.minor, tt.wantMajor, tt.wantMin)
		if got != tt.want {
			t.Errorf("versionAtLeast(%d.%d >= %d.%d): got %v, want %v",
				tt.major, tt.minor, tt.wantMajor, tt.wantMin, got, tt.want)
		}
	}
}

func TestSupportsContainerStopTimeoutBoundary(t *testing.T) {
	// StopTimeout= in [Container] was added in Podman 5.0. Anything below
	// must fall back to PodmanArgs=--stop-timeout=5.
	tests := []struct {
		major, minor int
		want         bool
	}{
		{4, 9, false},
		{4, 99, false},
		{5, 0, true},
		{5, 8, true},
		{6, 0, true},
	}
	for _, tt := range tests {
		got := podmanVersionSupportsStopTimeout(tt.major, tt.minor)
		if got != tt.want {
			t.Errorf("%d.%d: got %v, want %v", tt.major, tt.minor, got, tt.want)
		}
	}
}
