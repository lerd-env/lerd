//go:build windows

package cli

import (
	"reflect"
	"testing"
)

func TestServer9pArgs(t *testing.T) {
	line := `"C:\Program Files\Podman\podman.exe" machine server9p --serve "C:\Users\John Doe:0000e36a-facb-11e6-bd58-64006a7986d3" 10664`
	args, ok := server9pArgs(line)
	if !ok || !reflect.DeepEqual(args, []string{"--serve", `C:\Users\John Doe:0000e36a-facb-11e6-bd58-64006a7986d3`, "10664"}) {
		t.Errorf("server9pArgs = %q, %v", args, ok)
	}
	for _, other := range []string{
		`C:\Podman\podman.exe machine start`,
		`C:\Podman\gvproxy.exe -listen vsock://x`,
		`C:\lerd\lerd.exe p9-serve --serve C:\:g 1`,
	} {
		if _, ok := server9pArgs(other); ok {
			t.Errorf("%q read as Podman's server9p", other)
		}
	}
}

func TestIsLerdP9Serve(t *testing.T) {
	if !isLerdP9Serve(`"C:\Users\me\AppData\Local\lerd\bin\lerd.exe" p9-serve --serve C:\:g 1`) {
		t.Error("lerd's own server not recognised")
	}
	if isLerdP9Serve(`C:\lerd\lerd.exe start`) || isLerdP9Serve(`C:\x\other.exe p9-serve`) {
		t.Error("something else read as lerd's 9p server")
	}
}

// PowerShell writes a lone object instead of a one-element array, and a null
// CommandLine for a process it may not read.
func TestParseProcessList(t *testing.T) {
	one, err := parseProcessList([]byte(`{"ProcessId":12,"CommandLine":"a b"}`))
	if err != nil || !reflect.DeepEqual(one, []winProcess{{PID: 12, CommandLine: "a b"}}) {
		t.Errorf("single object = %+v, %v", one, err)
	}
	many, err := parseProcessList([]byte(`[{"ProcessId":1,"CommandLine":null},{"ProcessId":2,"CommandLine":"x"}]`))
	if err != nil || !reflect.DeepEqual(many, []winProcess{{PID: 1}, {PID: 2, CommandLine: "x"}}) {
		t.Errorf("array = %+v, %v", many, err)
	}
	if none, err := parseProcessList([]byte("  ")); err != nil || len(none) != 0 {
		t.Errorf("empty output = %+v, %v", none, err)
	}
}
