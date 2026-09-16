package nativephp

import "testing"

// The dashboard asks about every site on every poll. Answering with a TCP dial
// into the FastCGI port hands the connection to a child, which resets the
// ondemand idle timer, so a pool that should fall to zero never does. Loaded is
// the launchd equivalent of "the container is running", which is what the
// container runtime reports for the same question.
func TestLoadedReadsTheJobNotThePort(t *testing.T) {
	prev := poolJobLoaded
	t.Cleanup(func() { poolJobLoaded = prev })

	var asked []string
	poolJobLoaded = func(label string) bool {
		asked = append(asked, label)
		return true
	}

	if !Loaded("8.5") {
		t.Error("a loaded pool must read as up")
	}
	if len(asked) != 1 || asked[0] != UnitLabel("8.5") {
		t.Errorf("asked = %v, want the 8.5 unit label", asked)
	}

	poolJobLoaded = func(string) bool { return false }
	if Loaded("8.5") {
		t.Error("an unloaded pool must read as down")
	}
}
