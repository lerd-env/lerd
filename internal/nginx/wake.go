package nginx

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// SiteWaking reports whether requests to the site still land in the wake hold:
// its vhost still hands them there, or it has been restored but nginx has not
// reloaded since, so a redirect then would bounce the client straight back.
func SiteWaking(domain string) bool {
	path := filepath.Join(config.NginxConfD(), domain+".conf")
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), WakeHoldPath) {
		return err == nil
	}
	return !ServesVhostWrittenAt(st.ModTime())
}
