package podman

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// lerd_devtools C source, compiled into every FPM image. Written into the
// build context so the Containerfile's COPY can pick it up; see
// devtoolsBuildBlock and buildFPMImage.
//
//go:embed devtools
var devtoolsSrcFS embed.FS

//go:embed devtoolsbridge
var devtoolsBridgeFS embed.FS

// DevtoolsIni returns the conf.d ini with runtime placeholders substituted.
// {{ DEVTOOLS_TARGET }} resolves through config.DevtoolsBridgeTarget (the same
// socket the debug bridge ships to). {{ DEVTOOLS_KINDS }} is the comma-separated
// set of event kinds the extension should capture.
func DevtoolsIni() (string, error) {
	b, err := devtoolsBridgeFS.ReadFile("devtoolsbridge/96-lerd-devtools.ini")
	if err != nil {
		return "", fmt.Errorf("devtools ini embed: %w", err)
	}
	out := strings.ReplaceAll(string(b), "{{ DEVTOOLS_TARGET }}", config.DevtoolsBridgeTarget())
	out = strings.ReplaceAll(out, "{{ DEVTOOLS_KINDS }}", "query")
	return out, nil
}

// EnsureDevtoolsAssets writes the devtools conf.d ini to its host path so the
// always-mounted FPM volume has a regular file (not a podman-auto-created dir)
// at the bind-mount source. Idempotent; runs regardless of Devtools.Enabled
// because the FPM quadlet always mounts the ini — the sentinel controls
// capture, exactly like the debug bridge.
func EnsureDevtoolsAssets() error {
	dir := config.DevtoolsAssetsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating devtools dir: %w", err)
	}
	ini, err := DevtoolsIni()
	if err != nil {
		return err
	}
	path := config.DevtoolsIniFile()
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return fmt.Errorf("removing stale devtools ini directory %s: %w", path, rmErr)
			}
		} else if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == ini {
			return nil
		}
	}
	if err := os.WriteFile(path, []byte(ini), 0644); err != nil {
		return fmt.Errorf("writing devtools ini %s: %w", path, err)
	}
	return nil
}

// SetDevtoolsWorkersFlag flips the sentinel that opts queue/scheduler worker
// queries into capture. Present = workers captured, absent = skipped. Like the
// enable flag, it sits under the dumps assets dir (mounted at
// /usr/local/etc/lerd) so no FPM restart is needed.
func SetDevtoolsWorkersFlag(enabled bool) error {
	flag := config.DevtoolsWorkersFlagFile()
	if enabled {
		if err := os.MkdirAll(filepath.Dir(flag), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(flag, []byte("1\n"), 0644); err != nil {
			return fmt.Errorf("writing devtools workers flag: %w", err)
		}
		return nil
	}
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing devtools workers flag: %w", err)
	}
	return nil
}

// writeDevtoolsSource copies the embedded extension source into ctxDir so the
// Containerfile's `COPY lerd-devtools …` finds it in the build context.
func writeDevtoolsSource(ctxDir string) error {
	dst := filepath.Join(ctxDir, "lerd-devtools")
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := devtoolsSrcFS.ReadDir("devtools")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := devtoolsSrcFS.ReadFile("devtools/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0644); err != nil {
			return err
		}
	}
	return nil
}

// devtoolsBuildBlock returns the Containerfile snippet that compiles and
// enables the lerd_devtools extension. It is layered on top of the final image
// in both build paths (not baked into the base template), so the base-image
// hash is unchanged and the prebuilt-base fast path keeps working — the .so
// just compiles as one extra ~40s layer. The build pulls in the Alpine
// toolchain and removes it in the same RUN so it adds no weight, and is wrapped
// so a compile failure degrades to "queries silently unavailable" rather than
// bricking the whole image, matching the SPX block.
func devtoolsBuildBlock() string {
	steps := "apk add --no-cache --virtual .lerd-build autoconf make g++ && " +
		"cd /tmp/lerd-devtools && phpize && ./configure --enable-lerd-devtools && make -j$(nproc) && make install && docker-php-ext-enable lerd_devtools && " +
		"apk del .lerd-build"
	return "COPY lerd-devtools /tmp/lerd-devtools\n" +
		"RUN { " + steps + "; } || true; \\\n" +
		"    rm -rf /tmp/lerd-devtools /var/cache/apk/*\n"
}
