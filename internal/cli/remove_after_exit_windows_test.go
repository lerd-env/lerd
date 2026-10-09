//go:build windows

package cli

import "testing"

func TestDeferredRemovalScript(t *testing.T) {
	cases := []struct {
		path string
		dir  bool
		want string
	}{
		{`C:\Users\me\AppData\Local\lerd`, true, `ping -n 4 127.0.0.1 >nul & rmdir /s /q "C:\Users\me\AppData\Local\lerd"`},
		{`C:\Users\me\lerd.exe`, false, `ping -n 4 127.0.0.1 >nul & del /f /q "C:\Users\me\lerd.exe"`},
	}
	for _, c := range cases {
		if got := deferredRemovalScript(c.path, c.dir); got != c.want {
			t.Errorf("deferredRemovalScript(%q, %v) = %q, want %q", c.path, c.dir, got, c.want)
		}
	}
}
