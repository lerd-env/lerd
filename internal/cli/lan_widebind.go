package cli

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/podman"
)

// lanWideBindHint is what a user can do about a widened bind: install the pf
// redirect, or failing that firewall gvproxy.
const lanWideBindHint = "run 'lerd install' to keep them on loopback, or block incoming connections for gvproxy in the macOS firewall"

// lanWideBindSummary names the units whose loopback ports the LAN can reach
// anyway. Empty when LAN exposure is on, since every bind is then meant to be
// wide, and when every loopback bind holds.
func lanWideBindSummary(wide map[string][]int, lanExposed bool) string {
	if lanExposed || len(wide) == 0 {
		return ""
	}
	units := make([]string, 0, len(wide))
	for _, name := range slices.Sorted(maps.Keys(wide)) {
		ports := make([]string, 0, len(wide[name]))
		for _, p := range wide[name] {
			ports = append(ports, strconv.Itoa(p))
		}
		units = append(units, fmt.Sprintf("%s (%s)", name, strings.Join(ports, ", ")))
	}
	return "ports below 1024 publish on every interface here, not 127.0.0.1, so LAN devices can reach " + strings.Join(units, ", ")
}

// currentLANWideBindSummary is lanWideBindSummary for the installed units.
func currentLANWideBindSummary(lanExposed bool) string {
	wide, err := podman.LoopbackPortsPublishedWide()
	if err != nil {
		return ""
	}
	return lanWideBindSummary(wide, lanExposed)
}
