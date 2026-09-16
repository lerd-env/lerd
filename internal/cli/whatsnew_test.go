package cli

import (
	"strings"
	"testing"
)

// whatsnew has to resolve the newest version the same way status and update do,
// which means following the beta line when the running build is itself a beta.
// It used to ask for the latest *stable* release, so on a beta install it
// compared 1.35.0-beta.3 against 1.34.3, decided nothing was newer, and said:
//
//	✓ you are on the latest version (1.35.0-beta.3)
//
// while lerd status on the same machine, seconds earlier, offered
// "Update available: v1.35.0-beta.4". Two commands, one install, opposite
// answers, and the one meant to explain the update denied there was one.
func TestWhatsnewTargetResolution(t *testing.T) {
	const stable = "v1.34.3"
	const beta = "v1.35.0-beta.4"

	cases := []struct {
		name    string
		current string
		want    string
	}{
		{"a beta install follows the beta line", "1.35.0-beta.3", beta},
		{"a stable install stays on stable", "1.34.2", stable},
		{"a checkout build of a beta still follows betas", "1.35.0-beta.3-10-gabc1234", beta},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			restore := setWhatsnewTargets(stable, beta)
			defer restore()

			got, err := whatsnewLatestFor(c.current)
			if err != nil {
				t.Fatalf("whatsnewLatestFor(%q): %v", c.current, err)
			}
			if got != c.want {
				t.Errorf("whatsnewLatestFor(%q) = %q, want %q", c.current, got, c.want)
			}
		})
	}
}

// The answer whatsnew gives has to agree with the one status gives, since they
// are the same question asked twice.
func TestWhatsnewAgreesWithTheUpdateNotice(t *testing.T) {
	restore := setWhatsnewTargets("v1.34.3", "v1.35.0-beta.4")
	defer restore()

	latest, err := whatsnewLatestFor("1.35.0-beta.3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(latest, "beta.4") {
		t.Fatalf("whatsnew resolved %q, so it would report no update while status offers beta.4", latest)
	}
}

// setWhatsnewTargets pins both release lookups for a test.
func setWhatsnewTargets(stable, beta string) func() {
	prevS, prevB := whatsnewLatestStable, whatsnewLatestBeta
	whatsnewLatestStable = func() (string, error) { return stable, nil }
	whatsnewLatestBeta = func() (string, error) { return beta, nil }
	return func() { whatsnewLatestStable, whatsnewLatestBeta = prevS, prevB }
}
