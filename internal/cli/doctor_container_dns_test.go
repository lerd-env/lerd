package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestContainerDNSProbeHost(t *testing.T) {
	// The default store lives on GitHub, so that is what a stock install probes.
	if got := containerDNSProbeHost(); got != "raw.githubusercontent.com" {
		t.Fatalf("default probe host = %q, want raw.githubusercontent.com", got)
	}

	// A self-hosted store is what that install actually depends on, so probe it
	// rather than a hostname lerd never contacts.
	t.Setenv("LERD_STORE_BASE_URL", "https://store.internal.example/frameworks")
	if got := containerDNSProbeHost(); got != "store.internal.example" {
		t.Fatalf("self-hosted probe host = %q, want store.internal.example", got)
	}
}

func TestContainerDNSProbeHostIgnoresAPortAndAnUnparseableEntry(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", "https://store.internal.example:8443/frameworks")
	if got := containerDNSProbeHost(); got != "store.internal.example" {
		t.Fatalf("probe host = %q, want the port stripped", got)
	}

	t.Setenv("LERD_STORE_BASE_URL", "::::not a url")
	if got := containerDNSProbeHost(); got != "" {
		t.Fatalf("unparseable base = %q, want empty so the check skips", got)
	}
}

func TestCheckContainerExternalDNS(t *testing.T) {
	cases := []struct {
		name        string
		containerUp bool
		resolveErr  error
		wantStatus  string
		wantIn      string
	}{
		{
			name:        "resolves",
			containerUp: true,
			wantStatus:  "ok",
		},
		{
			// The exact shape of #1519: composer runs in the container, so a
			// resolver that works on the host proves nothing.
			name:        "cannot resolve",
			containerUp: true,
			resolveErr:  errors.New("exit status 2"),
			wantStatus:  "fail",
			wantIn:      "composer",
		},
		{
			name:        "no container to ask",
			containerUp: false,
			wantStatus:  "warn",
			wantIn:      "skipped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := &DoctorReport{}
			r := &dnsProbeRecorder{}
			checkContainerExternalDNS(containerDNSProbe{
				report:      rep,
				ok:          r.ok,
				fail:        r.fail,
				warn:        r.warn,
				containerUp: func(string) bool { return tc.containerUp },
				resolve:     func(string, string) error { return tc.resolveErr },
				host:        "store.example",
			})

			if r.status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (msg %q)", r.status, tc.wantStatus, r.msg)
			}
			if tc.wantIn != "" && !strings.Contains(strings.ToLower(r.msg+r.hint), tc.wantIn) {
				t.Fatalf("message %q + hint %q should mention %q", r.msg, r.hint, tc.wantIn)
			}
		})
	}
}

func TestCheckContainerExternalDNSSkipsWithNoProbeHost(t *testing.T) {
	r := &dnsProbeRecorder{}
	checkContainerExternalDNS(containerDNSProbe{
		report:      &DoctorReport{},
		ok:          r.ok,
		fail:        r.fail,
		warn:        r.warn,
		containerUp: func(string) bool { return true },
		resolve:     func(string, string) error { t.Fatal("must not probe without a host"); return nil },
		host:        "",
	})
	if r.status != "" {
		t.Fatalf("status = %q, want no finding at all", r.status)
	}
}

// dnsProbeRecorder captures the single finding the check reports.
type dnsProbeRecorder struct {
	status, label, msg, hint string
}

func (r *dnsProbeRecorder) ok(label string)        { r.status, r.label = "ok", label }
func (r *dnsProbeRecorder) warn(label, msg string) { r.status, r.label, r.msg = "warn", label, msg }
func (r *dnsProbeRecorder) fail(label, msg, hint string) {
	r.status, r.label, r.msg, r.hint = "fail", label, msg, hint
}

func TestApplyDoctorFixRoutesContainerDNS(t *testing.T) {
	called := false
	orig := resyncContainerDNSFn
	resyncContainerDNSFn = func() error { called = true; return nil }
	t.Cleanup(func() { resyncContainerDNSFn = orig })

	var out strings.Builder
	if err := ApplyDoctorFix(autoFix(fixContainerDNS, "", "re-point"), &out); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !called {
		t.Fatal("the container-dns fix must re-sync the network, not shell into another command")
	}
}

// The auto tier promises never to elevate, so this repair must stay off the
// heavy list and out of the sudo-gated set.
func TestContainerDNSFixIsNeitherHeavyNorPrivileged(t *testing.T) {
	if IsHeavyFix(autoFix(fixContainerDNS, "", "re-point")) {
		t.Error("re-pointing DNS destroys nothing, so it should not need re-confirmation")
	}
	if fixContainerDNS == fixWSLSetup || fixContainerDNS == fixDNSRepair {
		t.Error("the container-dns fix must not be one of the sudo-gated keys")
	}
}
