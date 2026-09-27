// Package winshim is the platform-neutral logic behind lerd.exe, the Windows
// side of a WSL install: the command shim that runs lerd inside the distro, and
// the DNS answers its login-time agent gives Windows for the site TLD. It is
// kept apart from cmd/lerd-windows so it is tested on Linux like the rest.
package winshim

import (
	"fmt"
	"strings"
)

// ConfigName is the file lerd.exe reads from its own directory. wsl:setup
// writes it, since lerd.exe cannot ask the distro without knowing which one.
const ConfigName = "lerd-wsl.conf"

// Config tells lerd.exe which distro runs lerd and where the binary is.
type Config struct {
	Distro string // WSL distro name, as `wsl -l` prints it
	Lerd   string // absolute path of the lerd binary inside the distro
	TLD    string // site TLD the agent answers for; empty when DNS is off
}

func (c Config) String() string {
	return fmt.Sprintf("distro=%s\nlerd=%s\ntld=%s\n", c.Distro, c.Lerd, c.TLD)
}

// ParseConfig reads the key=value file wsl:setup writes. An unknown key is
// ignored so a newer lerd can add one without breaking an older lerd.exe.
func ParseConfig(s string) (Config, error) {
	var c Config
	for _, l := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		switch k {
		case "distro":
			c.Distro = v
		case "lerd":
			c.Lerd = v
		case "tld":
			c.TLD = v
		}
	}
	if c.Distro == "" || c.Lerd == "" {
		return Config{}, fmt.Errorf("%s needs distro= and lerd=, run `lerd wsl:setup` inside WSL again", ConfigName)
	}
	return c, nil
}

// LinuxPath turns a \\wsl.localhost\<distro>\... (or \\wsl$\...) path into the
// path inside that distro. ok is false for any other path.
func LinuxPath(winPath, distro string) (string, bool) {
	for _, host := range []string{`\\wsl.localhost\`, `\\wsl$\`} {
		prefix := host + distro
		if len(winPath) < len(prefix) || !strings.EqualFold(winPath[:len(prefix)], prefix) {
			continue
		}
		rest := winPath[len(prefix):]
		if rest != "" && rest[0] != '\\' {
			continue // a different distro whose name starts the same
		}
		return "/" + strings.TrimLeft(strings.ReplaceAll(rest, `\`, "/"), "/"), true
	}
	return "", false
}

// WSLArgs builds wsl.exe's arguments to run lerd in cwd. A folder inside the
// distro is translated here rather than trusting wsl.exe to map a UNC path; a
// drive path is left for wsl.exe, which maps C:\ to /mnt/c itself.
func WSLArgs(c Config, cwd string, args []string) []string {
	if p, ok := LinuxPath(cwd, c.Distro); ok {
		cwd = p
	}
	return append([]string{"-d", c.Distro, "--cd", cwd, "--exec", c.Lerd}, args...)
}
