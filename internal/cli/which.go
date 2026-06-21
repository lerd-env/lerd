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
	"github.com/spf13/cobra"
)

// NewWhichCmd returns the which command.
func NewWhichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which",
		Short: "Show resolved PHP, Node, document root, and nginx config for the current site",
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

	phpVersion, _ := phpDet.DetectVersion(cwd)
	nodeVersion, _ := nodeDet.DetectVersion(cwd)

	publicDir := site.PublicDir
	if publicDir == "" {
		if fw, ok := config.GetFramework(site.Framework); ok && fw.PublicDir != "" {
			publicDir = fw.PublicDir
		} else {
			publicDir = "public"
		}
	}

	docRoot := filepath.Join(site.Path, publicDir)
	nginxConf := filepath.Join(config.NginxConfD(), site.PrimaryDomain()+".conf")

	sum := feedback.NewSummary().
		Row("Site", feedback.Val(strings.Join(site.Domains, ", "))).
		Row("PHP", phpVersion).
		Row("Node", nodeVersion).
		Row("Document root", docRoot).
		Row("Nginx config", nginxConf)
	if site.Secured {
		sslConf := filepath.Join(config.NginxConfD(), site.PrimaryDomain()+"-ssl.conf")
		sum.Row("Nginx SSL", sslConf)
	}
	sum.Print()

	return nil
}
