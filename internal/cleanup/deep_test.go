package cleanup

import (
	"os"
	"path/filepath"
	"testing"
)

// Catalog repos and protected refs are keyed by canonical form (the same
// registry.ParseImage canonicalisation deepTargets applies to image names).
func TestDeepTargets_ReapsUnusedServiceImagesKeepsProtected(t *testing.T) {
	imgs := []image{
		{Names: []string{"docker.io/library/mysql:8.4"}, Size: 500},   // current → protected
		{Names: []string{"docker.io/library/mysql:5.7"}, Size: 400},   // unused old → reap
		{Names: []string{"docker.io/library/redis:7"}, Size: 100},     // one-back rollback → protected
		{Names: []string{"docker.io/library/postgres:16"}, Size: 900}, // repo not in catalog → keep
		{Names: []string{"ubuntu:22.04"}, Size: 700},                  // user's own image → keep
		// multi-tag image: the rolling alias is unprotected but the pinned tag is
		// in use, so the whole image must be kept (per-image protection)
		{Names: []string{"docker.io/library/redis:7-alpine", "docker.io/library/redis:7.4.9-alpine"}, Size: 40},
	}
	repos := map[string]bool{"docker.io/library/mysql": true, "docker.io/library/redis": true}
	protected := map[string]bool{
		"docker.io/library/mysql:8.4":          true,
		"docker.io/library/redis:7":            true,
		"docker.io/library/redis:7.4.9-alpine": true,
	}
	pulled := map[string]bool{"docker.io/library/mysql:5.7": true}

	got := deepTargets(imgs, repos, protected, pulled, ScopeManaged)

	if len(got) != 1 || got[0].ID != "docker.io/library/mysql:5.7" {
		t.Fatalf("want only mysql:5.7 reaped, got %+v", got)
	}
	if got[0].Bytes != 400 {
		t.Errorf("bytes = %d, want 400", got[0].Bytes)
	}
}

// A multi-tag image whose tags are ALL unprotected catalog refs must have every
// tag removed (so the image actually frees), with the reclaimable bytes credited
// exactly once. An image carrying a non-catalog tag is left entirely alone.
func TestDeepTargets_MultiTagRemovesAllTagsCreditsOnce(t *testing.T) {
	imgs := []image{
		// both tags catalog + unprotected → remove both, count bytes once
		{Names: []string{"docker.io/library/mysql:5.7", "docker.io/library/mysql:5.7.40"}, Size: 400},
		// catalog tag + a user's own tag → keep the whole image
		{Names: []string{"docker.io/library/mysql:8.0", "myown:tag"}, Size: 999},
	}
	repos := map[string]bool{"docker.io/library/mysql": true}
	protected := map[string]bool{}
	pulled := map[string]bool{"docker.io/library/mysql:5.7": true}

	got := deepTargets(imgs, repos, protected, pulled, ScopeManaged)

	removed := map[string]int64{}
	for _, tg := range got {
		removed[tg.ID] = tg.Bytes
	}
	if len(removed) != 2 ||
		!hasKey(removed, "docker.io/library/mysql:5.7") ||
		!hasKey(removed, "docker.io/library/mysql:5.7.40") {
		t.Fatalf("want both mysql:5.7 tags removed, none from the user-tagged image, got %+v", got)
	}
	var total int64
	for _, b := range removed {
		total += b
	}
	if total != 400 {
		t.Errorf("reclaimable credited = %d, want 400 (once for the image)", total)
	}
}

func hasKey(m map[string]int64, k string) bool { _, ok := m[k]; return ok }

// A catalog image a container still holds is kept even when no service config
// references it: podman can't remove an in-use image, so listing it would just
// report "Freed 0 B" every run.
func TestDeepTargets_KeepsInUseServiceImage(t *testing.T) {
	imgs := []image{
		{Names: []string{"docker.io/library/redis:7-alpine"}, Size: 40, Containers: 1},
	}
	repos := map[string]bool{"docker.io/library/redis": true}

	got := deepTargets(imgs, repos, map[string]bool{}, map[string]bool{}, ScopeManaged)

	if len(got) != 0 {
		t.Fatalf("an in-use service image must be kept, got %+v", got)
	}
}

// With the ledger gate on (managed tier), an unused catalog image lerd never
// recorded pulling is kept: the ledger, not the repo name, marks it as lerd's.
func TestDeepTargets_KeepsCatalogImageLerdNeverPulled(t *testing.T) {
	imgs := []image{
		{Names: []string{"docker.io/library/redis:6"}, Size: 300},
	}
	repos := map[string]bool{"docker.io/library/redis": true}

	got := deepTargets(imgs, repos, map[string]bool{}, map[string]bool{}, ScopeManaged)

	if len(got) != 0 {
		t.Fatalf("a catalog image lerd never pulled must be kept, got %+v", got)
	}
}

// The deep tier drops the ledger gate, recovering images podman auto-pulled on
// container start, and the catalog gate, recovering the stranded base layer of a
// custom container (golang:1.25 here). Protection and in-use still hold.
func TestDeepTargets_DeepReapsEveryUnusedImage(t *testing.T) {
	imgs := []image{
		{Names: []string{"docker.io/library/mysql:8.0"}, Size: 800},             // unused, no ledger → reap
		{Names: []string{"docker.io/library/golang:1.25"}, Size: 900},           // stranded build base → reap
		{Names: []string{"docker.io/library/mysql:8.4"}, Size: 500},             // protected → keep
		{Names: []string{"alpine:latest"}, Size: 10},                            // lerd tool image → keep
		{Names: []string{"docker.io/library/redis:7"}, Size: 40, Containers: 1}, // in use → keep
	}
	repos := map[string]bool{"docker.io/library/mysql": true, "docker.io/library/redis": true}
	protected := map[string]bool{
		"docker.io/library/mysql:8.4":     true,
		"docker.io/library/alpine:latest": true,
	}

	got := deepTargets(imgs, repos, protected, map[string]bool{}, ScopeDeep)

	byRef := map[string]Target{}
	for _, tg := range got {
		byRef[tg.ID] = tg
	}
	if len(byRef) != 2 {
		t.Fatalf("want mysql:8.0 and golang:1.25 reaped, got %+v", got)
	}
	if byRef["docker.io/library/mysql:8.0"].Desc != "unused service image" {
		t.Errorf("a catalog leftover should read as a service image, got %q", byRef["docker.io/library/mysql:8.0"].Desc)
	}
	if byRef["docker.io/library/golang:1.25"].Desc != "unused image" {
		t.Errorf("a non-catalog leftover should read as a plain unused image, got %q", byRef["docker.io/library/golang:1.25"].Desc)
	}
	if byRef["docker.io/library/golang:1.25"].Bytes != 900 {
		t.Errorf("bytes = %d, want 900", byRef["docker.io/library/golang:1.25"].Bytes)
	}
}

// The managed tier is what the watcher runs unattended, so widening the deep
// tier must not let it near an image outside lerd's catalog.
func TestDeepTargets_ManagedStillIgnoresNonCatalogImages(t *testing.T) {
	imgs := []image{
		{Names: []string{"docker.io/library/golang:1.25"}, Size: 900},
		{Names: []string{"ubuntu:22.04"}, Size: 700},
	}
	repos := map[string]bool{"docker.io/library/mysql": true}

	if got := deepTargets(imgs, repos, map[string]bool{}, map[string]bool{}, ScopeManaged); len(got) != 0 {
		t.Fatalf("managed tier must leave non-catalog images alone, got %+v", got)
	}
}

// The deep tier is additive: the safe tier never touches service images, and
// only --deep reaps the unused one, leaving the current image alone.
func TestInspect_DeepAppendsUnusedServiceImages(t *testing.T) {
	withImages(t, []image{
		{ID: "m57", Names: []string{"docker.io/library/mysql:5.7"}, Size: 400}, // unused → deep reap
		{ID: "m84", Names: []string{"docker.io/library/mysql:8.4"}, Size: 500}, // current → keep
	}, nil)
	serviceRepos = func() (map[string]bool, error) {
		return map[string]bool{"docker.io/library/mysql": true}, nil
	}
	protectedImages = func() (map[string]bool, error) {
		return map[string]bool{"docker.io/library/mysql:8.4": true}, nil
	}
	loadPulledImages = func() map[string]bool {
		return map[string]bool{"docker.io/library/mysql:5.7": true}
	}
	t.Cleanup(func() {
		serviceRepos = realServiceRepos
		protectedImages = realProtectedImages
	})

	safe, err := Inspect(ScopeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if len(safe.Targets) != 0 {
		t.Fatalf("safe tier must not touch service images, got %+v", safe.Targets)
	}

	deep, err := Inspect(ScopeDeep)
	if err != nil {
		t.Fatal(err)
	}
	if len(deep.Targets) != 1 || deep.Targets[0].ID != "docker.io/library/mysql:5.7" {
		t.Fatalf("deep tier should reap only mysql:5.7, got %+v", deep.Targets)
	}
}

func TestCanonRefAndRepo(t *testing.T) {
	refCases := map[string]string{
		"mysql:8.4":                   "docker.io/library/mysql:8.4",
		"docker.io/library/mysql:8.4": "docker.io/library/mysql:8.4",
		"getmeili/meilisearch:v1.7":   "docker.io/getmeili/meilisearch:v1.7",
		"ghcr.io/x/y:tag":             "ghcr.io/x/y:tag",
	}
	for in, want := range refCases {
		if got := canonRef(in); got != want {
			t.Errorf("canonRef(%q) = %q, want %q", in, got, want)
		}
	}
	if got := canonRepo("docker.io/library/mysql:8.4"); got != "docker.io/library/mysql" {
		t.Errorf("canonRepo = %q, want docker.io/library/mysql", got)
	}
}

// The deep tier reaps any unused image, so a stopped site's PHP-FPM image has
// to be protected by the quadlet scan or a cleanup would cost a full rebuild.
func TestRealInstalledServiceImages_IncludesPHPFPM(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "containers", "systemd")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	units := map[string]string{
		"lerd-mysql.container":     "[Container]\nImage=docker.io/library/mysql:8.4\n",
		"lerd-php84-fpm.container": "[Container]\nImage=lerd-php84-fpm:local\n",
		"other.container":          "[Container]\nImage=someone-else:tag\n",
	}
	for name, body := range units {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got := map[string]bool{}
	for _, img := range realInstalledServiceImages() {
		got[img] = true
	}
	if !got["lerd-php84-fpm:local"] {
		t.Errorf("an installed PHP-FPM image must be protected, got %v", got)
	}
	if !got["docker.io/library/mysql:8.4"] {
		t.Errorf("an installed service image must be protected, got %v", got)
	}
	if got["someone-else:tag"] {
		t.Errorf("a non-lerd quadlet must not be scanned, got %v", got)
	}
}

// "Used by lerd" has to mean the whole stack, not just the service catalog:
// nginx and a stripe-listen sidecar come from quadlets, and the probe and mc
// images from lerd's tool set, none of which the catalog knows about.
func TestUsedImage_CountsQuadletAndToolImages(t *testing.T) {
	repos := map[string]bool{"docker.io/library/mysql": true}
	protected := map[string]bool{
		"docker.io/library/nginx:alpine":     true,
		"docker.io/stripe/stripe-cli:latest": true,
		"docker.io/library/alpine:latest":    true,
	}
	cases := []struct {
		name string
		img  image
		want bool
	}{
		{"catalog service", image{Names: []string{"docker.io/library/mysql:8.4"}}, true},
		{"quadlet image", image{Names: []string{"docker.io/library/nginx:alpine"}}, true},
		{"worker sidecar", image{Names: []string{"docker.io/stripe/stripe-cli:latest"}}, true},
		{"tool image", image{Names: []string{"alpine:latest"}}, true},
		{"someone else's", image{Names: []string{"docker.io/library/golang:1.25"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := usedImage(c.img, repos, protected); ok != c.want {
				t.Errorf("usedImage(%v) counted = %v, want %v", c.img.Names, ok, c.want)
			}
		})
	}
}

// A dangling image has no tag left, so the breakdown has to fall back to the
// short ID rather than showing a blank row.
func TestUsedImage_NamesDanglingByShortID(t *testing.T) {
	img := image{ID: "abcdef0123456789", Labels: map[string]string{"dev.lerd.kind": "fpm"}, Size: 500}
	u, ok := usedImage(img, map[string]bool{}, map[string]bool{})
	if !ok {
		t.Fatal("a lerd-built image must be counted")
	}
	if u.Ref != shortID(img.ID) {
		t.Errorf("ref = %q, want the short ID %q", u.Ref, shortID(img.ID))
	}
}

// A site's workers run as plain containers rather than quadlets, so their
// images have to come from the container list or a stripe-listen sidecar reads
// as someone else's image on the dashboard.
func TestRealProtectedImages_IncludesWorkerContainerImages(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	containerImages = func() []string { return []string{"docker.io/stripe/stripe-cli:latest"} }
	t.Cleanup(func() { containerImages = podmanContainerImages })

	prot, err := realProtectedImages()
	if err != nil {
		t.Fatal(err)
	}
	if !prot[canonRef("docker.io/stripe/stripe-cli:latest")] {
		t.Errorf("a worker container's image must be protected, got %v", prot)
	}
}
