package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The dashboard adds, renames and removes domains through its own handlers
// rather than the CLI's, so the CLI fix for a domain being reported before every
// nginx worker has it did not reach the Manage Domains modal. A plain reload
// signals the master and returns while the previous generation of workers is
// still serving, which is what made an added domain answer 404 and a removed one
// keep answering 200.
func TestDashboardDomainActionsSettleTheReload(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	caseStart := regexp.MustCompile(`(?m)^\tcase "`)
	starts := caseStart.FindAllStringIndex(string(src), -1)

	seen := 0
	for i, at := range starts {
		end := len(src)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		block := string(src)[at[0]:end]
		name := regexp.MustCompile(`^\tcase "domain:(\w+)"`).FindStringSubmatch(block)
		if name == nil {
			continue
		}
		seen++
		if strings.Contains(block, "nginx.Reload()") {
			t.Errorf("domain:%s reloads without settling, so the modal reports a change the workers have not taken", name[1])
		}
		if !strings.Contains(block, "nginx.ReloadAndSettle()") {
			t.Errorf("domain:%s does not settle its reload", name[1])
		}
	}
	if seen != 3 {
		t.Fatalf("found %d domain: cases, expected add, edit and remove; this test is watching the wrong shape", seen)
	}
}
