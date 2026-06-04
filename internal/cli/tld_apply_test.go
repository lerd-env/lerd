package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestParseTLDFlag(t *testing.T) {
	cfg := &config.GlobalConfig{}
	cfg.DNS.Enabled = true
	cfg.DNS.TLD = "test"

	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"", []string{"test"}, false},
		{"local", []string{"local"}, false},
		{"test,local", []string{"test", "local"}, false},
		{"test, local , lab", []string{"test", "local", "lab"}, false},
		{".test,.local", []string{"test", "local"}, false},
		{"test,test", []string{"test"}, false}, // de-duped
		{"with.dot", nil, true},                // invalid label
		{"localhost", nil, true},               // rejected with DNS enabled
	}
	for _, c := range cases {
		got, err := parseTLDFlag(c.in, cfg)
		if c.wantErr != (err != nil) {
			t.Errorf("parseTLDFlag(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseTLDFlag(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseTLDFlag(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestIntroducedTLDs(t *testing.T) {
	pre := map[string]bool{"test": true}
	got := introducedTLDs(pre, []string{"alice.test", "alice.local", "alice.lab", "x.local"})
	// .test already active; .local and .lab are new (each once).
	want := map[string]bool{"local": true, "lab": true}
	if len(got) != len(want) {
		t.Fatalf("introducedTLDs = %v, want keys %v", got, want)
	}
	for _, tld := range got {
		if !want[tld] {
			t.Errorf("unexpected introduced TLD %q", tld)
		}
	}
}
