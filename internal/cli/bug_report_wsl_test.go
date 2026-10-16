package cli

import "testing"

// The Windows account can differ from the distro's, so the WSL section hides
// its profile in both spellings and its bare name on its own.
func TestScrubWindowsProfile(t *testing.T) {
	in := `── /mnt/c/Users/Ana Pérez/.wslconfig
log: C:\Users\Ana Pérez\AppData\Local\lerd\bin\lerd-agent.log
owner Ana Pérez`
	want := `── %USERPROFILE%/.wslconfig
log: %USERPROFILE%\AppData\Local\lerd\bin\lerd-agent.log
owner %WINUSER%`
	if got := scrubWindowsProfile(in, "/mnt/c/Users/Ana Pérez", `C:\Users\Ana Pérez`, "ana"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := scrubWindowsProfile("C:\\Users\\x\\a", "", "", "ana"); got != "C:\\Users\\x\\a" {
		t.Errorf("without a profile nothing changes, got %q", got)
	}
}

// With the same name on both sides scrubHomePath's $USER covers it, so a
// Linux path keeps reading as $HOME instead of a Windows placeholder.
func TestScrubWindowsProfile_SameUserLeftToHomeScrub(t *testing.T) {
	got := scrubWindowsProfile("lerd=/home/ana/.local/bin/lerd", "/mnt/c/Users/ana", `C:\Users\ana`, "ana")
	if got != "lerd=/home/ana/.local/bin/lerd" {
		t.Errorf("got %q", got)
	}
}
