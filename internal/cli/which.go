package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	nodeDet "github.com/geodro/lerd/internal/node"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/webcustom"
	"github.com/spf13/cobra"
)

// NewWhichCmd returns the which command.
// whichPHPVersion reports the version a linked site actually runs on. The vhost
// is generated from the registry, so that is the answer; re-detecting made which
// disagree with every other surface as soon as a newer PHP was built, and could
// name a prerelease nothing serves from. Detection only fills in for a site
// linked before lerd recorded one.
func whichPHPVersion(site *config.Site, detected string) string {
	if site != nil && site.PHPVersion != "" {
		return site.PHPVersion
	}
	return detected
}

func NewWhichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which",
		Short: "Show resolved PHP, Node, document root, and web server config for the current site",
		RunE:  runWhich,
	}
}

func runWhich(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		return fmt.Errorf("no site registered for %s — link it first with lerd link", cwd)
	}

	detected, _ := phpDet.DetectVersion(cwd)
	phpVersion := whichPHPVersion(site, detected)
	nodeVersion, _ := nodeDet.DetectVersion(cwd)

	publicDir := config.PublicDirFor(*site)

	docRoot := filepath.Join(site.Path, publicDir)
	serverLabel, serverConf := whichServerRow(*site)

	sum := feedback.NewSummary().
		Row("Site", feedback.Val(strings.Join(site.Domains, ", "))).
		Row("PHP", phpVersion).
		Row("Node", nodeVersion).
		Row("Document root", docRoot).
		Row(serverLabel, serverConf)
	if site.Secured && serverLabel == "Nginx config" {
		sslConf := filepath.Join(config.NginxConfD(), site.PrimaryDomain()+"-ssl.conf")
		sum.Row("Nginx SSL", sslConf)
	}
	sum.Print()

	return nil
}

// whichServerRow names the file that configures the site on the server that
// serves it: the nginx vhost, or lerd's own web server's settings file.
func whichServerRow(site config.Site) (label, path string) {
	if cfg, err := config.LoadGlobal(); err == nil {
		if kind, _ := cfg.WebServerKind(); kind == config.WebServerGo {
			return "Web server settings", webcustom.SitePath(site.PrimaryDomain())
		}
	}
	return "Nginx config", filepath.Join(config.NginxConfD(), site.PrimaryDomain()+".conf")
}
