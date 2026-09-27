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
