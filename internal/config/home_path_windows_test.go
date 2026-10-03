//go:build windows

package config

import "testing"

// On Windows a path uses the native backslash, so a home of C:\Users\me has to
// contain C:\Users\me\AppData\... for the container to be trusted to see it.
func TestPathWithinNativeSeparator(t *testing.T) {
	for _, c := range []struct {
		p, root string
		want    bool
	}{
		{`C:\Users\me\AppData\Local\lerd\bin\composer.phar`, `C:\Users\me`, true},
		{`C:\Users\me`, `C:\Users\me`, true},
		{`C:\Users\me\site`, `C:\Users\me\`, true},
		{`C:\Users\meX\x`, `C:\Users\me`, false},
		{`D:\Sites\app`, `C:\Users\me`, false},
	} {
		if got := PathWithin(c.p, c.root); got != c.want {
			t.Errorf("PathWithin(%q, %q) = %v, want %v", c.p, c.root, got, c.want)
		}
	}
}
