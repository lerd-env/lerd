package winshim

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, r := range u {
		b[2*i], b[2*i+1] = byte(r), byte(r>>8)
	}
	return b
}

func TestParseDistroList_UTF16(t *testing.T) {
	// `wsl -l -q` writes UTF-16LE with CRLF line ends.
	got := ParseDistroList(utf16le("Ubuntu-24.04\r\ndocker-desktop\r\n\r\n"))
	want := []string{"Ubuntu-24.04", "docker-desktop"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWSLVersionReported(t *testing.T) {
	cases := map[string]struct {
		out  []byte
		want bool
	}{
		"english":     {utf16le("WSL version: 2.4.13.0\r\nKernel version: 5.15.167.4-1\r\n"), true},
		"spanish":     {utf16le("Versión de WSL: 3.0.1.0\r\n\r\nVersión de kernel: 6.18.40.1-1\r\n"), true},
		"utf8":        {[]byte("WSL version: 2.4.13.0\n"), true},
		"stub":        {utf16le("Windows Subsystem for Linux is not installed. You can install by running 'wsl.exe --install'.\r\n"), false},
		"kernel only": {utf16le("Kernel version: 5.15.167.4-1\r\n"), false},
		"empty":       {nil, false},
	}
	for name, c := range cases {
		if got := WSLVersionReported(c.out); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func TestParseDistroList_PlainUTF8(t *testing.T) {
	// With WSL_UTF8=1 set in the environment wsl.exe writes UTF-8 instead.
	got := ParseDistroList([]byte("Ubuntu-24.04\n"))
	if !reflect.DeepEqual(got, []string{"Ubuntu-24.04"}) {
		t.Errorf("got %q", got)
	}
}

func TestLinuxUsername(t *testing.T) {
	cases := map[string]string{
		"dumit":      "dumit",
		"George":     "george",
		"John Smith": "johnsmith",
		"Ana-Maria":  "ana-maria",
		"1user":      "u1user",
		"Ștefan":     "tefan",
		"":           "lerd",
	}
	for in, want := range cases {
		if got := LinuxUsername(in); got != want {
			t.Errorf("LinuxUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWSLMSIURL(t *testing.T) {
	release := []byte(`{"tag_name":"2.7.14","assets":[
		{"name":"wsl.2.7.14.0.arm64.msi","browser_download_url":"https://x/arm64.msi"},
		{"name":"wsl.2.7.14.0.x64.msi","browser_download_url":"https://x/x64.msi"},
		{"name":"Microsoft.WSL_2.7.14.0_x64_ARM64.msixbundle","browser_download_url":"https://x/bundle"}]}`)
	for arch, want := range map[string]string{"amd64": "https://x/x64.msi", "arm64": "https://x/arm64.msi"} {
		got, err := WSLMSIURL(release, arch)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", arch, got, err, want)
		}
	}
	if _, err := WSLMSIURL([]byte(`{"assets":[]}`), "amd64"); err == nil {
		t.Error("a release without the MSI must be an error")
	}
}
