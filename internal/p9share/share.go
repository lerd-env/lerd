// Package p9share serves the host folders a Hyper-V Podman machine mounts over
// 9p. Podman's own `podman machine server9p` leaks a Windows handle on every
// lookup and cannot replace an existing file, append, lock or set times, which
// breaks composer, SQLite and Laravel's log on a project folder. lerd runs the
// same server built on a fixed hugelgupf/p9 in its place.
package p9share

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hugelgupf/p9/fsimpl/localfs"
	"github.com/hugelgupf/p9/p9"
)

// Share is one served folder and the hvsock service GUID the VM connects to.
type Share struct {
	Dir     string
	Service string
}

// ParseServerArgs reads a `podman machine server9p` argument list: one or more
// --serve DIR:GUID followed by the PID the server lives as long as. Anything
// else is refused, so a Podman release that changes the format is noticed.
func ParseServerArgs(args []string) ([]Share, int, error) {
	var shares []Share
	pid := -1
	for i := 0; i < len(args); i++ {
		a := args[i]
		var spec string
		switch {
		case a == "--serve":
			if i+1 >= len(args) {
				return nil, 0, fmt.Errorf("--serve without a value")
			}
			i++
			spec = args[i]
		case strings.HasPrefix(a, "--serve="):
			spec = strings.TrimPrefix(a, "--serve=")
		case strings.HasPrefix(a, "-"):
			return nil, 0, fmt.Errorf("unknown server9p flag %q", a)
		default:
			if pid >= 0 {
				return nil, 0, fmt.Errorf("unexpected server9p argument %q", a)
			}
			n, err := strconv.Atoi(a)
			if err != nil || n < 0 {
				return nil, 0, fmt.Errorf("server9p PID %q is not a number", a)
			}
			pid = n
			continue
		}
		cut := strings.LastIndex(spec, ":")
		if cut <= 0 || cut == len(spec)-1 {
			return nil, 0, fmt.Errorf("share %q has no hvsock GUID", spec)
		}
		if _, err := Port(spec[cut+1:]); err != nil {
			return nil, 0, fmt.Errorf("share %q: %w", spec, err)
		}
		shares = append(shares, Share{Dir: spec[:cut], Service: spec[cut+1:]})
	}
	if len(shares) == 0 {
		return nil, 0, fmt.Errorf("server9p serves no folders")
	}
	if pid < 0 {
		return nil, 0, fmt.Errorf("server9p names no PID")
	}
	return shares, pid, nil
}

// ServerArgs is the inverse of ParseServerArgs, the arguments `lerd p9-serve`
// takes for the same shares.
func ServerArgs(shares []Share, pid int) []string {
	var out []string
	for _, s := range shares {
		out = append(out, "--serve", s.Dir+":"+s.Service)
	}
	return append(out, strconv.Itoa(pid))
}

// Port is the vsock port an hvsock service GUID carries in its first field, the
// number the VM's client9p dials.
func Port(service string) (uint32, error) {
	head, _, ok := strings.Cut(service, "-")
	if !ok || len(head) != 8 || len(service) != 36 {
		return 0, fmt.Errorf("%q is not an hvsock service GUID", service)
	}
	n, err := strconv.ParseUint(head, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not an hvsock service GUID", service)
	}
	return uint32(n), nil
}

// serve answers 9p on l for dir until l is closed. dir must be an absolute path
// to an existing folder, as Podman requires of its own server.
func serve(l net.Listener, dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("shared folder %s is not an absolute path", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("shared folder %s is not a directory", dir)
	}
	return p9.NewServer(localfs.Attacher(dir)).Serve(l)
}
