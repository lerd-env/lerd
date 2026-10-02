//go:build windows

package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/geodro/lerd/internal/feedback"
)

// maxClockSkew is how far the Windows clock may drift from the machine's before
// lerd says so; below it the difference is mostly the ssh round trip.
const maxClockSkew = time.Minute

// warnOnClockSkew compares the Windows clock with the machine's, which keeps
// its own time over NTP. Files written from Windows carry the host's time and
// files written in containers the machine's, so a drift between them makes
// template engines skip recompiling an edited view, and S3 refuses requests.
func warnOnClockSkew(machine string) {
	out, err := machineQuery("machine", "inspect", "--format", "{{.ConnectionInfo.PodmanPipe.Path}}", machine)
	if err != nil {
		return
	}
	vm, err := vmClock(strings.TrimSpace(string(out)))
	if err != nil {
		return
	}
	if msg := clockSkewWarning(time.Now(), vm); msg != "" {
		feedback.Warn("%s", msg)
	}
}

// vmClock reads the machine's time off the Date header the Podman service in
// the VM puts on its API responses, a few milliseconds over the named pipe.
func vmClock(pipe string) (time.Time, error) {
	if pipe == "" {
		return time.Time{}, fmt.Errorf("the machine has no API pipe")
	}
	client := http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return winio.DialPipeContext(ctx, pipe)
		},
	}}
	resp, err := client.Get("http://d/_ping")
	if err != nil {
		return time.Time{}, err
	}
	resp.Body.Close()
	return http.ParseTime(resp.Header.Get("Date"))
}

// clockSkewWarning describes the drift between host and vm, or "" when it is
// within maxClockSkew.
func clockSkewWarning(host, vm time.Time) string {
	skew, dir := vm.Sub(host), "behind"
	if skew < 0 {
		skew, dir = -skew, "ahead of"
	}
	if skew <= maxClockSkew {
		return ""
	}
	return fmt.Sprintf("the Windows clock is %s %s the Podman machine's; edited views may not recompile and S3 requests are refused past 15m, sync the Windows time (Settings > Time & language > Sync now)",
		skew.Round(time.Second), dir)
}
