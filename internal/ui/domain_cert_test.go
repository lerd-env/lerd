package ui

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
)

// stubPodmanOnPath puts a no-op podman first on PATH, so a reload never shells
// out to real podman and leaves container storage in the test's tmp dir that
// the runner can't delete.
func stubPodmanOnPath(t *testing.T) {
	t.Helper()
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "podman"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// makeWorktree fakes a git worktree at <sitePath>/.git/worktrees/<name>
// pointing at <checkoutPath>. Matches what real `git worktree add` writes
// so gitpkg.DetectWorktrees picks it up.
func makeWorktree(t *testing.T, sitePath, name, branch, checkoutPath string) {
	t.Helper()
	wtDir := filepath.Join(sitePath, ".git", "worktrees", name)
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checkoutPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, "gitdir"), []byte(filepath.Join(checkoutPath, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupSecuredSite primes XDG, global config, and registers a
// secured site at sitePath with the given domains. Returns the site struct
// so the test can call config.AddSite again after mutating Domains, etc.
func setupSecuredSite(t *testing.T, primary string, extras ...string) (sitePath string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanOnPath(t)

	cfg := &config.GlobalConfig{}
	cfg.DNS.Enabled = true
	cfg.DNS.TLD = "test"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	sitePath = filepath.Join(tmp, "project")
	if err := os.MkdirAll(sitePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sitePath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	domains := append([]string{primary + ".test"}, extras...)
	site := config.Site{
		Name:    strings.TrimSuffix(primary, ".test"),
		Path:    sitePath,
		Domains: domains,
		Secured: true,
	}
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	return sitePath
}

// readSiteSANs returns the DNS names of <XDG_DATA_HOME>/lerd/certs/sites/<primary>.crt.
func readSiteSANs(t *testing.T, primary string) []string {
	t.Helper()
	path := filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd", "certs", "sites", primary+".crt")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading cert %q: %v", path, err)
	}
	block, _ := pem.Decode(body)
	if block == nil {
		t.Fatalf("cert %q is not PEM", path)
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return leaf.DNSNames
}

// Adding a domain to a secured site must reissue the cert so the new SAN
// covers the new hostname AND so any existing worktree subdomains stay in
// the SAN list. Regression pin for two related bugs:
//
//  1. The handler used certs.IssueCert (a documented no-op when the cert
//     file already exists), so the new domain never made it into the SAN
//     and the browser rejected it with ERR_CERT_AUTHORITY_INVALID.
//  2. After switching to certs.IssueCertForce directly, the worktree
//     subdomain (<branch>.<primary>) silently dropped out of the SAN
//     because that bypass skipped the worktree-aware helper.
//
// Catching either regression requires asserting all three SAN classes
// here: new alias, existing primary, and worktree subdomain.
func TestHandleSiteAction_domainAdd_keepsWorktreeSANs(t *testing.T) {
	sitePath := setupSecuredSite(t, "harborlist")
	makeWorktree(t, sitePath, "main", "main", filepath.Join(t.TempDir(), "wt-main"))

	// Pre-issue the cert so it already exists on disk when domain:add
	// runs. Without this precondition a regression to bare certs.IssueCert
	// would still write a fresh cert with the new SAN (because no prior
	// file exists to short-circuit on) and the test would miss the bug.
	if err := certs.ReissueCertForWorktree(config.Site{
		Name: "harborlist", Path: sitePath,
		Domains: []string{"harborlist.test"}, Secured: true,
	}); err != nil {
		t.Fatalf("pre-issue cert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/harborlist.test/domain:add?name=aaaddd", nil)
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok response, got %s", rec.Body.String())
	}

	sans := readSiteSANs(t, "harborlist.test")
	for _, san := range []string{
		"harborlist.test", "*.harborlist.test",
		"aaaddd.test", "*.aaaddd.test",
		"main.harborlist.test", "*.main.harborlist.test",
	} {
		if !slices.Contains(sans, san) {
			t.Errorf("SAN %q missing after domain:add; SANs: %v", san, sans)
		}
	}
}

// Removing a domain must reissue the cert so the SAN list drops the
// removed alias but keeps the worktree subdomains. Same regression shape
// as domain:add — the original handler used the cert-skipping IssueCert,
// and the first attempted fix dropped worktree SANs. Pin both.
func TestHandleSiteAction_domainRemove_keepsWorktreeSANs(t *testing.T) {
	sitePath := setupSecuredSite(t, "harborlist", "aaaddd.test")
	makeWorktree(t, sitePath, "main", "main", filepath.Join(t.TempDir(), "wt-main"))

	if err := certs.ReissueCertForWorktree(config.Site{
		Name: "harborlist", Path: sitePath,
		Domains: []string{"harborlist.test", "aaaddd.test"}, Secured: true,
	}); err != nil {
		t.Fatalf("pre-issue cert: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/harborlist.test/domain:remove?name=aaaddd", nil)
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok response, got %s", rec.Body.String())
	}

	sans := readSiteSANs(t, "harborlist.test")
	for _, san := range []string{
		"harborlist.test", "*.harborlist.test",
		"main.harborlist.test", "*.main.harborlist.test",
	} {
		if !slices.Contains(sans, san) {
			t.Errorf("SAN %q missing after domain:remove; SANs: %v", san, sans)
		}
	}
	for _, gone := range []string{"aaaddd.test", "*.aaaddd.test"} {
		if slices.Contains(sans, gone) {
			t.Errorf("SAN %q should have been dropped after domain:remove; SANs: %v", gone, sans)
		}
	}
}
