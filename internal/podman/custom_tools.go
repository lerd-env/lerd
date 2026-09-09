package podman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// A custom container is built from the project's own Containerfile, so what it
// carries is the project's business. PHP tooling may only be routed into it
// when it actually has PHP: the `container:` section serves Node, Python and Go
// sites too, and execing php into a node image fails with an OCI runtime error
// rather than anything the user can act on.

// customTools is the cached answer for one site's image.
type customTools struct {
	ImageID string `json:"image_id"`
	PHP     bool   `json:"php"`
}

// probeCustomPHPFn is the probe itself, a seam so the cache can be tested
// without building an image.
var probeCustomPHPFn = func(image string) bool {
	return RunSilent("run", "--rm", "--entrypoint", "php", PlatformImage(image), "--version") == nil
}

func customToolsPath(siteName string) string {
	return filepath.Join(config.DataDir(), "container-tools", siteName+".json")
}

// CustomImageHasPHP reports whether a site's custom image carries a php binary.
// The answer is probed once and cached against the image ID, since a probe
// starts a container and every `lerd php` in the project would otherwise pay
// for one. A rebuilt image changes the ID and is probed again.
func CustomImageHasPHP(siteName string) bool {
	image := CustomImageName(siteName)
	id := customImageID(image)
	if id == "" {
		return false
	}
	if cached, ok := readCustomTools(siteName); ok && cached.ImageID == id {
		return cached.PHP
	}
	has := probeCustomPHPFn(image)
	writeCustomTools(siteName, customTools{ImageID: id, PHP: has})
	return has
}

// customImageID returns the local image's ID, or "" when it isn't built yet.
func customImageID(image string) string {
	out, err := Run("image", "inspect", PlatformImage(image), "--format", "{{.Id}}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func readCustomTools(siteName string) (customTools, bool) {
	data, err := os.ReadFile(customToolsPath(siteName))
	if err != nil {
		return customTools{}, false
	}
	var c customTools
	if json.Unmarshal(data, &c) != nil {
		return customTools{}, false
	}
	return c, true
}

// writeCustomTools stores the answer, and stays quiet when it cannot: a cache
// that fails to persist costs a probe next time, nothing more.
func writeCustomTools(siteName string, c customTools) {
	path := customToolsPath(siteName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

// ForgetCustomImageTools drops the cached answer for a site, so a removed or
// rebuilt image is never answered for from a stale file.
func ForgetCustomImageTools(siteName string) {
	_ = os.Remove(customToolsPath(siteName))
}
