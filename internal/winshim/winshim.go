// Package winshim is the platform-neutral logic behind lerd.exe, the Windows
// side of a WSL install: the command shim that runs lerd inside the distro, and
// the DNS answers its login-time agent gives Windows for the site TLD. It is
// kept apart from cmd/lerd-windows so it is tested on Linux like the rest.
package winshim

import (
	"fmt"
	"path"
	"regexp"
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
	Shims  string // lerd's shim directory inside the distro; empty before wsl:setup wrote it
	Mount  string // where WSL mounts Windows drives, /mnt unless wsl.conf moves it
}

func (c Config) String() string {
	return fmt.Sprintf("distro=%s\nlerd=%s\ntld=%s\nshims=%s\nmount=%s\n", c.Distro, c.Lerd, c.TLD, c.Shims, c.Mount)
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
		case "shims":
			c.Shims = v
		case "mount":
			c.Mount = v
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
	return wslArgs(c, cwd, c.Lerd, args)
}

// ShimFromExe names the shim a copy of lerd.exe stands for, php for php.exe,
// or "" for lerd.exe itself and the lerd-setup.exe installer.
func ShimFromExe(exe string) string {
	base := exe[strings.LastIndexAny(exe, `\/`)+1:]
	name := strings.TrimSuffix(strings.ToLower(base), ".exe")
	if strings.HasPrefix(name, "lerd") {
		return ""
	}
	return name
}

// ShimArgs builds wsl.exe's arguments to run one of lerd's shims, such as php
// or composer, from cwd: what php.cmd and its siblings on the Windows PATH run.
func ShimArgs(c Config, cwd, name string, args []string) ([]string, error) {
	if !shimName.MatchString(name) {
		return nil, fmt.Errorf("%q is not a lerd shim name", name)
	}
	if c.Shims == "" {
		return nil, fmt.Errorf("%s does not name lerd's shim directory, run `lerd wsl:setup` inside WSL again", ConfigName)
	}
	return wslArgs(c, cwd, path.Join(c.Shims, name), args), nil
}

func wslArgs(c Config, cwd, exe string, args []string) []string {
	if p, ok := LinuxPath(cwd, c.Distro); ok {
		cwd = p
	}
	out := []string{"-d", c.Distro, "--cd", cwd, "--exec", exe}
	for _, a := range args {
		out = append(out, ArgToLinux(c, a))
	}
	return out
}

var (
	drivePath = regexp.MustCompile(`^([A-Za-z]):[\\/]`)
	shimName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ArgToLinux turns an argument that is a Windows path, on a drive or inside
// this distro, into the path lerd sees, so an IDE passing the file it acts on
// reaches the right one. A --flag=path value is translated the same way, and
// anything else, a PHP namespace included, is left as it is.
func ArgToLinux(c Config, arg string) string {
	if flag, val, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(flag, "-") {
		return flag + "=" + ArgToLinux(c, val)
	}
	if p, ok := LinuxPath(arg, c.Distro); ok {
		return p
	}
	m := drivePath.FindStringSubmatch(arg)
	if m == nil {
		return arg
	}
	mount := c.Mount
	if mount == "" {
		mount = "/mnt"
	}
	return path.Join(mount, strings.ToLower(m[1]), strings.ReplaceAll(arg[3:], `\`, "/"))
}
