package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// maxSiteSweepConcurrency bounds how many sites the doctor diagnoses at once.
// The quick pass is file reads and unit-state probes, so this is about not
// stampeding the service manager rather than about CPU.
const maxSiteSweepConcurrency = 8

// siteSweepResult is one site's line in the doctor's site sweep.
type siteSweepResult struct {
	Label    string
	Failures int
	Warnings int
	Summary  string
}

// sitesForSweep is a seam tests replace, so the sweep can be exercised without a
// site registry on the machine running the tests.
var sitesForSweep = func() []config.Site {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	return reg.Sites
}

// quickSiteReport is the per-site diagnosis, hooked for the same reason.
var quickSiteReport = func(path, fwName string) sitedoctor.Response {
	return sitedoctor.RunQuickForPath(context.Background(), path, fwName)
}

// sweepSites runs the cheap half of the site doctor over every linked site,
// skipping the ignored ones and those with nothing to check. Results keep the
// registry's order so two runs on an unchanged machine read the same.
func sweepSites() []siteSweepResult {
	var targets []config.Site
	for _, s := range sitesForSweep() {
		if s.Ignored || !sitedoctor.AppliesForPath(s.Path, s.Framework) {
			continue
		}
		targets = append(targets, s)
	}

	out := make([]siteSweepResult, len(targets))
	sem := make(chan struct{}, maxSiteSweepConcurrency)
	var wg sync.WaitGroup
	for i, s := range targets {
		wg.Add(1)
		go func(i int, s config.Site) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = summariseSiteReport(siteSweepLabel(s), quickSiteReport(s.Path, s.Framework))
		}(i, s)
	}
	wg.Wait()
	return out
}

// siteSweepLabel is what the user types after `lerd site:doctor`, so the pointer
// in the finding is a command they can paste.
func siteSweepLabel(s config.Site) string {
	if d := s.PrimaryDomain(); d != "" {
		return d
	}
	return s.Name
}

// summariseSiteReport reduces a site's report to one line: the counts, and the
// first failing check named so the user knows what they are about to look at.
func summariseSiteReport(label string, resp sitedoctor.Response) siteSweepResult {
	res := siteSweepResult{Label: label, Failures: resp.Failures, Warnings: resp.Warnings}
	if res.Failures == 0 && res.Warnings == 0 {
		return res
	}
	var parts []string
	if res.Failures > 0 {
		parts = append(parts, fmt.Sprintf("%d failing", res.Failures))
	}
	if res.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", res.Warnings))
	}
	summary := strings.Join(parts, ", ")
	if first := firstProblem(resp); first != "" {
		summary += " (" + first + ")"
	}
	res.Summary = summary
	return res
}

// firstProblem names the first failing check, falling back to the first warning.
func firstProblem(resp sitedoctor.Response) string {
	for _, want := range []string{sitedoctor.StatusFail, sitedoctor.StatusWarn} {
		for _, c := range resp.Checks {
			if c.Status != want {
				continue
			}
			if c.Label != "" {
				return c.Label
			}
			return c.Name
		}
	}
	return ""
}
