package winshim

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

// DecodeWSLOutput turns wsl.exe output into a string. wsl.exe writes UTF-16LE
// unless WSL_UTF8 is set, and the inbox stub ignores that, so both are read.
func DecodeWSLOutput(out []byte) string {
	if len(out) >= 2 && len(out)%2 == 0 && out[1] == 0 {
		u := make([]uint16, len(out)/2)
		for i := range u {
			u[i] = uint16(out[2*i]) | uint16(out[2*i+1])<<8
		}
		return string(utf16.Decode(u))
	}
	return string(out)
}

// ParseDistroList reads the names `wsl -l -q` prints.
func ParseDistroList(out []byte) []string {
	var names []string
	for _, l := range strings.Split(DecodeWSLOutput(out), "\n") {
		if l = strings.TrimSpace(strings.Trim(l, "\x00")); l != "" {
			names = append(names, l)
		}
	}
	return names
}

// LinuxUsername derives a valid Linux login from a Windows user name: lower
// case ASCII letters, digits and dashes, starting with a letter.
func LinuxUsername(windows string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(windows) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	name := b.String()
	if name == "" {
		return "lerd"
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "u" + name
	}
	return name
}

// WSLMSIURL picks the WSL installer for arch out of a GitHub release JSON from
// microsoft/WSL. The MSI rather than the Store package, because the Store
// route silently does nothing outside an interactive session.
func WSLMSIURL(releaseJSON []byte, arch string) (string, error) {
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(releaseJSON, &rel); err != nil {
		return "", err
	}
	suffix := map[string]string{"amd64": ".x64.msi", "arm64": ".arm64.msi"}[arch]
	for _, a := range rel.Assets {
		if suffix != "" && strings.HasSuffix(a.Name, suffix) {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("no WSL MSI for %s in the latest microsoft/WSL release", arch)
}
