package services

import (
	"reflect"
	"testing"
)

func TestSplitExecStartWindowsBackslashes(t *testing.T) {
	old := literalBackslashes
	literalBackslashes = true
	t.Cleanup(func() { literalBackslashes = old })

	cases := []struct {
		line string
		want []string
	}{
		{`C:\Users\me\lerd.exe watch`, []string{`C:\Users\me\lerd.exe`, "watch"}},
		{`"C:\Program Files\lerd\lerd.exe" ui`, []string{`C:\Program Files\lerd\lerd.exe`, "ui"}},
		{`podman exec -w 'C:\Sites\my shop' fpm`, []string{"podman", "exec", "-w", `C:\Sites\my shop`, "fpm"}},
		{`echo 'it'\''s'`, []string{"echo", "it's"}},
	}
	for _, c := range cases {
		if got := SplitExecStart(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitExecStart(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
