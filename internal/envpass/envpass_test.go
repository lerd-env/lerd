package envpass

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNames(t *testing.T) {
	environ := []string{
		"APP_KEY=base64:abc",
		"DB_PASSWORD=secret",
		"STRIPE_SECRET_KEY=sk_test",
		"STRIPE_WEBHOOK_SECRET=whsec",
		"PATH=/usr/bin",
		"HOME=/home/dev",
		"COMPOSER_HOME=/home/dev/.composer",
		"LERD_SITE=other",
		"LD_PRELOAD=/evil.so",
		"not an identifier=1",
		"UNASKED=1",
	}

	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"no patterns forwards nothing", nil, nil},
		{"exact names", []string{"APP_KEY", "DB_PASSWORD"}, []string{"APP_KEY", "DB_PASSWORD"}},
		{"glob prefix", []string{"STRIPE_*"}, []string{"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET"}},
		{"single char glob", []string{"APP_KE?"}, []string{"APP_KEY"}},
		{"unmatched names are skipped", []string{"NOT_SET", "APP_KEY"}, []string{"APP_KEY"}},
		{"container vars are denied", []string{"PATH", "HOME", "COMPOSER_HOME"}, nil},
		{"lerd and loader prefixes are denied", []string{"LERD_*", "LD_*"}, nil},
		{"a catch-all still cannot reach denied vars", []string{"*"}, []string{"APP_KEY", "DB_PASSWORD", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET", "UNASKED"}},
		{"overlapping patterns dedupe", []string{"STRIPE_*", "STRIPE_SECRET_KEY"}, []string{"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET"}},
		{"a pattern with shell metacharacters is ignored", []string{"$(id)", "APP_KEY"}, []string{"APP_KEY"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Names(tt.patterns, environ); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Names(%v) = %v, want %v", tt.patterns, got, tt.want)
			}
		})
	}
}

func TestPatternsFromProviderVar(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"comma separated", "APP_KEY,DB_PASSWORD", []string{"APP_KEY", "DB_PASSWORD"}},
		{"space separated", "APP_KEY DB_PASSWORD", []string{"APP_KEY", "DB_PASSWORD"}},
		{"stray separators", " APP_KEY , ,DB_PASSWORD ", []string{"APP_KEY", "DB_PASSWORD"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Patterns("", []string{EnvVar + "=" + tt.raw})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Patterns(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestPatternsMergesProjectConfig(t *testing.T) {
	dir := t.TempDir()
	yaml := "env_passthrough:\n  - STRIPE_*\n  - DB_PASSWORD\n"
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Patterns(dir, []string{EnvVar + "=APP_KEY"})
	want := []string{"APP_KEY", "STRIPE_*", "DB_PASSWORD"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Patterns = %v, want %v", got, want)
	}
}

func TestArgsForwardsNamesWithoutValues(t *testing.T) {
	environ := []string{EnvVar + "=APP_KEY,DB_PASSWORD", "APP_KEY=base64:abc", "DB_PASSWORD=secret"}
	got := Args("", environ)
	want := []string{"--env", "APP_KEY", "--env", "DB_PASSWORD"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Args = %v, want %v", got, want)
	}
	for _, a := range got {
		if a == "secret" || a == "base64:abc" {
			t.Fatal("a secret value reached the argument list")
		}
	}
}

func TestArgsEmptyWithoutRequest(t *testing.T) {
	if got := Args("", []string{"APP_KEY=base64:abc"}); got != nil && len(got) != 0 {
		t.Fatalf("Args = %v, want none", got)
	}
}
