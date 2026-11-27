package ui

import (
	"net/http"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

// handleSiteUnlinkPreview answers what deleting the site's directory would take
// with it. Only the lerd host itself may delete files, so only it may ask.
func handleSiteUnlinkPreview(w http.ResponseWriter, r *http.Request, domain string) {
	if !isLocalControlRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, siteops.ReportSiteDir(site.Path, site.PrimaryDomain()))
}

// unlinkOptions reads an unlink's delete_dir request, which needs the lerd host
// itself and the site's name typed back, or returns why it is refused.
func unlinkOptions(r *http.Request, name string) (cli.UnlinkOptions, string) {
	if r.URL.Query().Get("delete_dir") != "1" {
		return cli.UnlinkOptions{}, ""
	}
	if !isLocalControlRequest(r) {
		return cli.UnlinkOptions{}, "deleting a site's directory is only possible on the lerd host itself"
	}
	if r.URL.Query().Get("confirm") != name {
		return cli.UnlinkOptions{}, "type the site name to delete its directory"
	}
	return cli.UnlinkOptions{DeleteDir: true, Confirmed: true}, ""
}
