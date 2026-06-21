package certs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// issueCertMu serialises issueCertAtomic calls per primaryDomain. Two
// concurrent reissues for the same site (e.g. boot scanWorktrees racing the
// watcher's syncWorktree on the same site) must not interleave their
// renames — pre-fix both used a fixed "<primary>.crt.new" tempfile path,
// so one would clobber the other's tempfile or rename a partially-flushed
// file. Lock per domain so unrelated sites still issue in parallel.
var issueCertMu sync.Map // map[string]*sync.Mutex

func lockForDomain(domain string) *sync.Mutex {
	if m, ok := issueCertMu.Load(domain); ok {
		return m.(*sync.Mutex)
	}
	m, _ := issueCertMu.LoadOrStore(domain, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// tempSuffixSeq increments per call to issueCertAtomic so concurrent
// callers (across processes too) don't share a tempfile path even if the
// per-domain mutex is bypassed somehow.
var tempSuffixSeq atomic.Uint64

// MkcertPath returns the path to the mkcert binary.
func MkcertPath() string {
	return filepath.Join(config.BinDir(), "mkcert")
}

// InstallCA installs the mkcert root CA into the system trust store.
func InstallCA() error {
	cmd := exec.Command(MkcertPath(), "-install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mkcert -install: %w", err)
	}
	return nil
}

// IssueCert issues a TLS certificate covering all the given domains using mkcert.
// The cert files are named after primaryDomain. Each domain also gets a wildcard entry.
// If the cert and key files already exist they are reused without re-running mkcert.
func IssueCert(primaryDomain string, allDomains []string, certsDir string) error {
	certFile := filepath.Join(certsDir, primaryDomain+".crt")
	keyFile := filepath.Join(certsDir, primaryDomain+".key")
	if _, certErr := os.Stat(certFile); certErr == nil {
		if _, keyErr := os.Stat(keyFile); keyErr == nil {
			return nil
		}
	}
	return issueCertAtomic(primaryDomain, allDomains, certsDir)
}

// IssueCertForce regenerates the certificate for primaryDomain even if files
// exist. Writes to temp paths and renames atomically so a transient mkcert
// failure leaves the previous cert/key intact (which is critical: a missing
// cert trips RepairVhosts into flipping the site to plain HTTP).
func IssueCertForce(primaryDomain string, allDomains []string, certsDir string) error {
	return issueCertAtomic(primaryDomain, allDomains, certsDir)
}

func issueCertAtomic(primaryDomain string, allDomains []string, certsDir string) error {
	mu := lockForDomain(primaryDomain)
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(certsDir, 0755); err != nil {
		return err
	}

	certFile := filepath.Join(certsDir, primaryDomain+".crt")
	keyFile := filepath.Join(certsDir, primaryDomain+".key")
	// Per-call unique suffix: pid + monotonic seq + ns time. Ensures
	// cross-process concurrent issuers (e.g. lerd-watcher and lerd-ui)
	// don't collide on the .new path even when the in-process mutex
	// can't help.
	suffix := ".new." + strconv.Itoa(os.Getpid()) + "." + strconv.FormatUint(tempSuffixSeq.Add(1), 10) + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
	tmpCert := certFile + suffix
	tmpKey := keyFile + suffix

	var sans []string
	for _, d := range allDomains {
		sans = append(sans, d, "*."+d)
	}

	args := []string{"-cert-file", tmpCert, "-key-file", tmpKey}
	args = append(args, sans...)

	// Capture mkcert's chatty success banner instead of letting it spill into
	// the CLI's clean step output; surface it only when the command fails.
	cmd := exec.Command(MkcertPath(), args...)
	var mkOut bytes.Buffer
	cmd.Stdout = &mkOut
	cmd.Stderr = &mkOut
	if err := cmd.Run(); err != nil {
		os.Remove(tmpCert) //nolint:errcheck
		os.Remove(tmpKey)  //nolint:errcheck
		return fmt.Errorf("mkcert for %s: %w\n%s", primaryDomain, err, strings.TrimSpace(mkOut.String()))
	}

	// Two-phase rename: back up the previous cert (if any), swap in the
	// new cert, then swap in the new key. If the key rename fails the
	// cert is rolled back so we don't leave a cert/key mismatch on disk
	// — nginx with mismatched cert and key refuses to start the site,
	// which is worse than the transient mkcert failure we're guarding
	// against in the first place.
	bakCert := certFile + ".bak." + strconv.Itoa(os.Getpid())
	hadPrevCert := false
	if _, err := os.Stat(certFile); err == nil {
		if err := os.Rename(certFile, bakCert); err != nil {
			os.Remove(tmpCert) //nolint:errcheck
			os.Remove(tmpKey)  //nolint:errcheck
			return fmt.Errorf("backing up cert for %s: %w", primaryDomain, err)
		}
		hadPrevCert = true
	}
	if err := os.Rename(tmpCert, certFile); err != nil {
		os.Remove(tmpCert) //nolint:errcheck
		os.Remove(tmpKey)  //nolint:errcheck
		if hadPrevCert {
			os.Rename(bakCert, certFile) //nolint:errcheck
		}
		return fmt.Errorf("renaming cert for %s: %w", primaryDomain, err)
	}
	if err := os.Rename(tmpKey, keyFile); err != nil {
		os.Remove(tmpKey)   //nolint:errcheck
		os.Remove(certFile) //nolint:errcheck
		if hadPrevCert {
			os.Rename(bakCert, certFile) //nolint:errcheck
		}
		return fmt.Errorf("renaming key for %s: %w", primaryDomain, err)
	}
	if hadPrevCert {
		os.Remove(bakCert) //nolint:errcheck
	}
	return nil
}

// CertExists returns true if the certificate for the domain already exists.
func CertExists(domain string) bool {
	certFile := filepath.Join(config.CertsDir(), "sites", domain+".crt")
	_, err := os.Stat(certFile)
	return err == nil
}
