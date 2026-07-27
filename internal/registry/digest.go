package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// manifestDigestTTL bounds how long a digest read stays valid on disk. The
// answer only moves when a tag is republished (an upstream refresh under the
// same recipe hash), so a few hours of staleness costs nothing while keeping
// status rebuilds off the network.
const manifestDigestTTL = 6 * time.Hour

// manifestAccept asks for every manifest media type, index types first, so a
// multi-arch tag answers with the index digest rather than one platform's
// manifest. That is the digest podman records for a pulled multi-arch image,
// which is what callers compare against.
const manifestAccept = "application/vnd.oci.image.index.v1+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.docker.distribution.manifest.v2+json"

// digestFetchTimeout is deliberately shorter than fetchTimeout: a digest read
// sits behind interactive paths (doctor, a manual check), and the answer is
// optional, so waiting is worse than going without it.
const digestFetchTimeout = 5 * time.Second

type cachedDigest struct {
	FetchedAt time.Time `json:"fetched_at"`
	Digest    string    `json:"digest"`
}

func digestCachePath(ref ImageRef) string {
	h := sha256.Sum256([]byte(ref.Registry + "/" + ref.Repo + ":" + ref.Tag))
	return filepath.Join(cacheDir(), "digest-"+hex.EncodeToString(h[:])+".json")
}

// CachedManifestDigest returns a recent digest for image without touching the
// network. ok is false when nothing is cached, the entry expired, or the entry
// records a failed fetch.
func CachedManifestDigest(image string) (string, bool) {
	ref, err := ParseImage(image)
	if err != nil {
		return "", false
	}
	entry, ok := readDigestCache(ref)
	if !ok || entry.Digest == "" {
		return "", false
	}
	return entry.Digest, true
}

// ManifestDigest returns the digest the registry currently serves for image's
// tag. It is a manifest HEAD, so nothing is pulled. Results are cached on disk
// for manifestDigestTTL, failures included: an unreachable or unauthenticated
// registry is then retried at most once per window instead of on every check.
func ManifestDigest(image string) (string, error) {
	ref, err := ParseImage(image)
	if err != nil {
		return "", err
	}
	if entry, ok := readDigestCache(ref); ok {
		if entry.Digest == "" {
			return "", &UnreachableErr{Cause: fmt.Errorf("no digest for %s (cached failure)", image)}
		}
		return entry.Digest, nil
	}
	digest, err, _ := digestGroup.Do(digestCachePath(ref), func() (any, error) {
		d, fetchErr := fetchManifestDigest(ref)
		writeDigestCache(ref, d)
		if fetchErr != nil {
			return "", fetchErr
		}
		return d, nil
	})
	if err != nil {
		return "", err
	}
	return digest.(string), nil
}

// RefreshManifestDigest reads the digest straight from the registry, replacing
// whatever was cached. Build paths use it so an image is never stamped with a
// digest that has been superseded within the cache window.
func RefreshManifestDigest(image string) (string, error) {
	if err := InvalidateManifestDigest(image); err != nil {
		return "", err
	}
	return ManifestDigest(image)
}

// InvalidateManifestDigest drops the cached digest for image so the next read
// goes back to the registry. Used by the manual "check for updates" flow.
func InvalidateManifestDigest(image string) error {
	ref, err := ParseImage(image)
	if err != nil {
		return nil
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if err := os.Remove(digestCachePath(ref)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// digestGroup collapses concurrent reads of the same tag into one HEAD.
var digestGroup = newSingleflight()

func fetchManifestDigest(ref ImageRef) (string, error) {
	url := "https://" + ref.Registry + "/v2/" + ref.Repo + "/manifests/" + ref.Tag
	ctx, cancel := context.WithTimeout(context.Background(), digestFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAccept)
	// GHCR always wants a pull token, even for public repos. Other registries
	// are read anonymously; one that refuses simply yields no digest.
	if ref.Registry == "ghcr.io" {
		tok, tokErr := fetchGHCRToken(ref.Repo)
		if tokErr != nil {
			return "", tokErr
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", &UnreachableErr{Cause: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "", &AuthRequiredErr{Registry: ref.Registry}
	case resp.StatusCode == http.StatusNotFound:
		return "", &NotFoundErr{Repo: ref.Repo}
	case resp.StatusCode != http.StatusOK:
		return "", &UnreachableErr{Cause: fmt.Errorf("%s manifest %d", ref.Registry, resp.StatusCode)}
	}
	digest := strings.TrimSpace(resp.Header.Get("Docker-Content-Digest"))
	if digest == "" {
		return "", &UnreachableErr{Cause: fmt.Errorf("%s served no Docker-Content-Digest for %s", ref.Registry, ref.Tag)}
	}
	return digest, nil
}

func readDigestCache(ref ImageRef) (cachedDigest, bool) {
	data, err := os.ReadFile(digestCachePath(ref))
	if err != nil {
		return cachedDigest{}, false
	}
	var entry cachedDigest
	if err := json.Unmarshal(data, &entry); err != nil {
		return cachedDigest{}, false
	}
	if time.Since(entry.FetchedAt) > manifestDigestTTL {
		return cachedDigest{}, false
	}
	return entry, true
}

// writeDigestCache stores digest for ref, an empty string recording that the
// fetch failed.
func writeDigestCache(ref ImageRef, digest string) {
	data, err := json.Marshal(cachedDigest{FetchedAt: time.Now(), Digest: digest})
	if err != nil {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	writeCacheFile(digestCachePath(ref), data)
}
