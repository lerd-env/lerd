package hostpath

import "testing"

func TestToVM(t *testing.T) {
	cases := map[string]string{
		`C:\Sites\app`:          "/mnt/c/Sites/app",
		`c:\Sites\app\`:         "/mnt/c/Sites/app",
		"D:/data/x":             "/mnt/d/data/x",
		`C:\`:                   "/mnt/c",
		"/var/www/html":         "/var/www/html",
		"relative/dir":          "relative/dir",
		`C:\My Projects\shop 2`: "/mnt/c/My Projects/shop 2",
		"":                      "",
	}
	for in, want := range cases {
		if got := ToVM(in); got != want {
			t.Errorf("ToVM(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromVMIsTheInverse(t *testing.T) {
	cases := map[string]string{
		"/mnt/c/Sites/app": `C:\Sites\app`,
		"/mnt/d":           `D:\`,
		"/var/www/html":    "/var/www/html",
		"/mnt/cdrom/x":     "/mnt/cdrom/x",
	}
	for in, want := range cases {
		if got := FromVM(in); got != want {
			t.Errorf("FromVM(%q) = %q, want %q", in, got, want)
		}
	}
	for _, p := range []string{`C:\Sites\app`, `E:\a\b c`} {
		if got := FromVM(ToVM(p)); got != p {
			t.Errorf("round trip of %q gave %q", p, got)
		}
	}
}

func TestDriveRootIsConfigurable(t *testing.T) {
	old := DriveRoot
	DriveRoot = "/host"
	t.Cleanup(func() { DriveRoot = old })
	if got := ToVM(`C:\Sites`); got != "/host/c/Sites" {
		t.Errorf("got %q", got)
	}
	if got := FromVM("/host/c/Sites"); got != `C:\Sites` {
		t.Errorf("got %q", got)
	}
}

func TestIsWindowsPath(t *testing.T) {
	for p, want := range map[string]bool{`C:\x`: true, "c:/x": true, "C:": false, "/x": false, "": false, `\srv\share`: false} {
		if got := IsWindowsPath(p); got != want {
			t.Errorf("IsWindowsPath(%q) = %v, want %v", p, got, want)
		}
	}
}
