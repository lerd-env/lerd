// Package download is the shared helper behind every binary lerd fetches from
// the network: composer, fnm, mkcert, phpantom_lsp, and self-update archives.
// It retries transient failures with a short backoff and cancels stalled
// transfers, so a momentary CDN hiccup doesn't abort a whole install.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Variables rather than constants so tests can shorten them.
var (
	attempts    = 3
	retryDelay  = 2 * time.Second
	idleTimeout = 60 * time.Second
)

// File downloads url into dest with the given mode, printing a progress bar
// to w. Network errors, HTTP 5xx, 408 and 429 are retried with a short pause;
// any other failure is returned immediately. An attempt that stops receiving
// bytes for idleTimeout is cancelled into the retry path instead of hanging.
func File(ctx context.Context, url, dest string, mode os.FileMode, w io.Writer) error {
	return Verified(ctx, url, dest, mode, "", w)
}

// Verified is File with an expected sha256 of the downloaded bytes. An empty
// digest skips the check, for a tool the manifest pins no hash for. Bytes land
// in a temporary file beside dest and are only renamed over it once they have
// been read whole and matched, so a failed or tampered download can never
// replace a working binary with a truncated one.
func Verified(ctx context.Context, url, dest string, mode os.FileMode, sha256Hex string, w io.Writer) error {
	fmt.Fprintf(w, "\n      Downloading %s\n      ", url)

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			fmt.Fprintf(w, "\n      Retrying after transient failure: %v\n      ", lastErr)
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(retryDelay):
			}
		}
		retryable, err := fetchOnce(ctx, url, dest, mode, sha256Hex, w)
		if err == nil {
			return nil
		}
		if !retryable || ctx.Err() != nil {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// fetchOnce runs a single download attempt. The bool reports whether the
// failure is transient and worth retrying.
func fetchOnce(ctx context.Context, url, dest string, mode os.FileMode, sha256Hex string, w io.Writer) (bool, error) {
	actx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The watchdog cancels the attempt when nothing has arrived for
	// idleTimeout; progressReader pushes it forward on every read.
	watchdog := time.AfterFunc(idleTimeout, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(actx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return retryableStatus(resp.StatusCode), fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".part-*")
	if err != nil {
		return false, err
	}
	tmp := f.Name()
	defer func() {
		f.Close()
		os.Remove(tmp) // no-op once the rename below has moved it
	}()

	sum := sha256.New()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, w: w,
		progressed: func() { watchdog.Reset(idleTimeout) }}
	written, err := io.Copy(io.MultiWriter(f, sum), pr)
	if err != nil {
		return true, err
	}
	if err := f.Close(); err != nil {
		return true, err
	}
	if sha256Hex != "" {
		if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, sha256Hex) {
			// Not retryable: the bytes arrived intact and are the wrong ones.
			return false, fmt.Errorf("checksum mismatch for %s: got %s, expected %s", url, got, sha256Hex)
		}
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return false, err
	}
	fmt.Fprintf(w, " (%d bytes)\n", written)
	return false, nil
}

// retryableStatus reports whether a status is a transient server-side
// failure: any 5xx, plus request timeout and rate limiting.
func retryableStatus(code int) bool {
	return code >= 500 || code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
}

// progressReader mirrors the stream into a terminal progress bar and tells
// the stall watchdog that bytes are still flowing.
type progressReader struct {
	r          io.Reader
	total      int64
	written    int64
	w          io.Writer
	progressed func()
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.progressed()
	}
	p.written += int64(n)
	if p.total > 0 {
		pct := int(float64(p.written) / float64(p.total) * 50)
		bar := ""
		for i := 0; i < 50; i++ {
			if i < pct {
				bar += "="
			} else {
				bar += " "
			}
		}
		fmt.Fprintf(p.w, "\r      [%s] %d%%", bar, pct*2)
	}
	return n, err
}
