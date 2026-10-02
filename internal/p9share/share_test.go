package p9share

import (
	"reflect"
	"testing"
)

func TestParseServerArgs(t *testing.T) {
	args := []string{
		"--serve", `C:\Users\me:0000e36a-facb-11e6-bd58-64006a7986d3`,
		"--serve=C:\\:0000e36b-facb-11e6-bd58-64006a7986d3",
		"--serve", `C:\Users\John Doe\AppData\Roaming\containers:0000e36c-facb-11e6-bd58-64006a7986d3`,
		"10664",
	}
	shares, pid, err := ParseServerArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	want := []Share{
		{Dir: `C:\Users\me`, Service: "0000e36a-facb-11e6-bd58-64006a7986d3"},
		{Dir: `C:\`, Service: "0000e36b-facb-11e6-bd58-64006a7986d3"},
		{Dir: `C:\Users\John Doe\AppData\Roaming\containers`, Service: "0000e36c-facb-11e6-bd58-64006a7986d3"},
	}
	if !reflect.DeepEqual(shares, want) || pid != 10664 {
		t.Errorf("ParseServerArgs = %+v, %d", shares, pid)
	}
	if got := ServerArgs(shares, pid); !reflect.DeepEqual(got, []string{
		"--serve", want[0].Dir + ":" + want[0].Service,
		"--serve", want[1].Dir + ":" + want[1].Service,
		"--serve", want[2].Dir + ":" + want[2].Service,
		"10664",
	}) {
		t.Errorf("ServerArgs does not round-trip: %q", got)
	}
}

// An argument list lerd does not understand is refused, so a Podman release
// that changes server9p leaves its own server running instead of a broken one.
func TestParseServerArgsRefusesWhatItDoesNotKnow(t *testing.T) {
	for name, args := range map[string][]string{
		"no shares":     {"10664"},
		"no pid":        {"--serve", `C:\:0000e36b-facb-11e6-bd58-64006a7986d3`},
		"bad pid":       {"--serve", `C:\:0000e36b-facb-11e6-bd58-64006a7986d3`, "x"},
		"no guid":       {"--serve", `C:\Users`, "1"},
		"unknown flag":  {"--readonly", "--serve", `C:\:0000e36b-facb-11e6-bd58-64006a7986d3`, "1"},
		"two pids":      {"--serve", `C:\:0000e36b-facb-11e6-bd58-64006a7986d3`, "1", "2"},
		"dangling flag": {"1", "--serve"},
	} {
		if _, _, err := ParseServerArgs(args); err == nil {
			t.Errorf("%s: %q accepted", name, args)
		}
	}
}

func TestPort(t *testing.T) {
	if p, err := Port("0000e36a-facb-11e6-bd58-64006a7986d3"); err != nil || p != 58218 {
		t.Errorf("Port = %d, %v; want 58218", p, err)
	}
	if _, err := Port("zz"); err == nil {
		t.Error("a malformed GUID should fail")
	}
}
