// Package download is the shared helper behind every binary lerd fetches from
// the network: composer, fnm, mkcert, phpantom_lsp, and self-update archives.
// It retries transient failures with a short backoff and cancels stalled
// transfers, so a momentary CDN hiccup doesn't abort a whole install.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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
		retryable, err := fetchOnce(ctx, url, dest, mode, w)
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
func fetchOnce(ctx context.Context, url, dest string, mode os.FileMode, w io.Writer) (bool, error) {
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

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return false, err
	}
	defer f.Close()

	pr := &progressReader{r: resp.Body, total: resp.ContentLength, w: w,
		progressed: func() { watchdog.Reset(idleTimeout) }}
	written, err := io.Copy(f, pr)
	if err != nil {
		return true, err
	}
	fmt.Fprintf(w, " (%d bytes)\n", written)

	return false, os.Chmod(dest, mode)
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
