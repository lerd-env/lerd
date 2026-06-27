package podman

import "testing"

func TestUpgradeNeedsHeal(t *testing.T) {
	cases := []struct {
		name     string
		prev     PodmanEnv
		cur      PodmanEnv
		wantHeal bool
	}{
		{
			name:     "first install, no prior fingerprint",
			prev:     PodmanEnv{},
			cur:      PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			wantHeal: false,
		},
		{
			name:     "major upgrade 3 to 4",
			prev:     PodmanEnv{Version: "3.4.4", Major: 3, Minor: 4, NetworkBackend: "cni"},
			cur:      PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			wantHeal: true,
		},
		{
			name:     "major downgrade also heals",
			prev:     PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			cur:      PodmanEnv{Version: "3.4.4", Major: 3, Minor: 4, NetworkBackend: "cni"},
			wantHeal: true,
		},
		{
			name:     "minor bump within same major does not heal",
			prev:     PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			cur:      PodmanEnv{Version: "4.9.3", Major: 4, Minor: 9, NetworkBackend: "netavark"},
			wantHeal: false,
		},
		{
			name:     "backend switch within same major heals",
			prev:     PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "cni"},
			cur:      PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			wantHeal: true,
		},
		{
			name:     "unknown backend on either side never triggers a backend heal",
			prev:     PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: ""},
			cur:      PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"},
			wantHeal: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := upgradeNeedsHeal(tc.prev, tc.cur)
			if got != tc.wantHeal {
				t.Fatalf("upgradeNeedsHeal = %v (%q), want %v", got, reason, tc.wantHeal)
			}
			if got && reason == "" {
				t.Error("heal needed but reason was empty")
			}
		})
	}
}

func TestCleanVersionToken(t *testing.T) {
	cases := map[string]string{
		"4.6.2":     "4.6.2",
		"4.9.3+ds1": "4.9.3",
		"5.8.2-rc1": "5.8.2",
		"3.4":       "3.4",
	}
	for in, want := range cases {
		if got := cleanVersionToken(in); got != want {
			t.Errorf("cleanVersionToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPodmanEnvRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := LoadPodmanEnv(); got.Version != "" {
		t.Fatalf("expected zero env before any save, got %+v", got)
	}
	want := PodmanEnv{Version: "4.6.2", Major: 4, Minor: 6, NetworkBackend: "netavark"}
	if err := SavePodmanEnv(want); err != nil {
		t.Fatalf("SavePodmanEnv: %v", err)
	}
	if got := LoadPodmanEnv(); got != want {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
}
