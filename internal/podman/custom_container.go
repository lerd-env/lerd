package podman

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// CustomContainerName returns the Podman container name for a site's custom
// container, e.g. "lerd-custom-nestapp".
func CustomContainerName(siteName string) string {
	return "lerd-custom-" + siteName
}

// CustomImageName returns the local image tag for a site's custom container,
// e.g. "lerd-custom-nestapp:local".
func CustomImageName(siteName string) string {
	return CustomContainerName(siteName) + ":local"
}

// ResolveContainerfile returns the absolute path to the Containerfile for a
// project. If cfg specifies a Containerfile it is resolved relative to
// projectPath; otherwise the default "Containerfile.lerd" is used.
func ResolveContainerfile(projectPath string, cfg *config.ContainerConfig) string {
	name := "Containerfile.lerd"
	if cfg != nil && cfg.Containerfile != "" {
		name = cfg.Containerfile
	}
	return filepath.Join(projectPath, name)
}

// ResolveBuildContext returns the build context directory for a custom
// container build. Defaults to the project root.
func ResolveBuildContext(projectPath string, cfg *config.ContainerConfig) string {
	if cfg != nil && cfg.BuildContext != "" && cfg.BuildContext != "." {
		return filepath.Join(projectPath, cfg.BuildContext)
	}
	return projectPath
}

// HasContainerfile returns true when the project directory contains a
// Containerfile.lerd (the default custom container definition).
func HasContainerfile(projectPath string) bool {
	_, err := os.Stat(filepath.Join(projectPath, "Containerfile.lerd"))
	return err == nil
}

// CustomImageExists returns true when the local image for a site's custom
// container is present in the podman store.
func CustomImageExists(siteName string) bool {
	return ImageExists(CustomImageName(siteName))
}

// CustomImageUpToDate returns true when the image exists and the stored
// Containerfile hash matches the current file on disk. Returns false when
// the image is missing, the hash file is missing, or the Containerfile
// has changed since the last build.
func CustomImageUpToDate(siteName, projectPath string, cfg *config.ContainerConfig) bool {
	if !CustomImageExists(siteName) {
		return false
	}
	stored := readContainerfileHash(siteName)
	if stored == "" {
		return false
	}
	current := hashContainerfile(projectPath, cfg)
	return current != "" && current == stored
}

// StoreContainerfileHash persists the MD5 hash of the current Containerfile
// so subsequent link calls can skip the build when the file hasn't changed.
func StoreContainerfileHash(siteName, projectPath string, cfg *config.ContainerConfig) {
	hash := hashContainerfile(projectPath, cfg)
	if hash == "" {
		return
	}
	dir := filepath.Join(config.DataDir(), "container-hashes")
	os.MkdirAll(dir, 0755)                                         //nolint:errcheck
	os.WriteFile(filepath.Join(dir, siteName), []byte(hash), 0644) //nolint:errcheck
}

// RemoveContainerfileHash removes the stored hash for a site.
func RemoveContainerfileHash(siteName string) {
	path := filepath.Join(config.DataDir(), "container-hashes", siteName)
	os.Remove(path) //nolint:errcheck
}

func hashContainerfile(projectPath string, cfg *config.ContainerConfig) string {
	path := ResolveContainerfile(projectPath, cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	target := ""
	if cfg != nil {
		target = cfg.Target
	}
	ctx, err := hashOwnBuildContext(projectPath, cfg)
	if err != nil {
		return ""
	}
	// Mix the build target into the hash so flipping target: development to
	// target: production in .lerd.yaml invalidates the cache. Without this,
	// CustomImageUpToDate would return a stale image when only the target
	// changed (issue #379).
	return fmt.Sprintf("%x", md5.Sum([]byte(string(data)+"\x00--target="+target+ctx)))
}

// hashOwnBuildContext hashes the files of a build context kept in a directory of
// its own. One holding the project, the default, returns "": hashing it would
// rebuild the image on every source edit.
func hashOwnBuildContext(projectPath string, cfg *config.ContainerConfig) (string, error) {
	root := ResolveBuildContext(projectPath, cfg)
	rel, err := filepath.Rel(root, projectPath)
	if err != nil {
		return "", err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil
	}
	h := md5.New()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%s\x00", name, info.Mode())
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00", target)
		case info.Mode().IsRegular():
			fmt.Fprintf(h, "%d\x00", info.Size())
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("\x00--context=%x", h.Sum(nil)), nil
}

func readContainerfileHash(siteName string) string {
	path := filepath.Join(config.DataDir(), "container-hashes", siteName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// CustomBuildFailed reports whether the site's last image build failed, which
// leaves it serving the image built before.
func CustomBuildFailed(siteName string) bool {
	_, err := os.Stat(customBuildFailurePath(siteName))
	return err == nil
}

// ClearCustomBuildFailure forgets a recorded failure.
func ClearCustomBuildFailure(siteName string) {
	os.Remove(customBuildFailurePath(siteName)) //nolint:errcheck
}

func customBuildFailurePath(siteName string) string {
	return filepath.Join(config.DataDir(), "container-build-failures", siteName)
}

func recordCustomBuild(siteName string, err error) {
	if err == nil {
		ClearCustomBuildFailure(siteName)
		return
	}
	path := customBuildFailurePath(siteName)
	os.MkdirAll(filepath.Dir(path), 0755)              //nolint:errcheck
	os.WriteFile(path, []byte(err.Error()+"\n"), 0644) //nolint:errcheck
}

// buildCustomImageArgs assembles the podman build argv for a custom site
// image. Centralised so BuildCustomImage and BuildCustomImageTo stay in
// sync and the --target plumbing only lives in one place.
func buildCustomImageArgs(imageName, containerfile, buildCtx string, cfg *config.ContainerConfig) []string {
	args := []string{"build", "-t", imageName, "-f", containerfile}
	if cfg != nil && cfg.Target != "" {
		args = append(args, "--target", cfg.Target)
	}
	return append(args, buildCtx)
}

// BuildCustomImage builds the OCI image for a site's custom container from
// the user's Containerfile. The image is tagged as lerd-custom-{siteName}:local.
func BuildCustomImage(siteName, projectPath string, cfg *config.ContainerConfig) (err error) {
	defer func() { recordCustomBuild(siteName, err) }()
	containerfile := ResolveContainerfile(projectPath, cfg)
	if _, err := os.Stat(containerfile); err != nil {
		return fmt.Errorf("containerfile not found: %s", containerfile)
	}

	buildCtx := ResolveBuildContext(projectPath, cfg)
	imageName := CustomImageName(siteName)

	cmd := execCommand(PodmanBin(), buildCustomImageArgs(imageName, containerfile, buildCtx, cfg)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building custom image %s: %w", imageName, err)
	}
	return nil
}

// BuildCustomImageTo builds the custom image, writing progress to w.
func BuildCustomImageTo(siteName, projectPath string, cfg *config.ContainerConfig, w interface{ Write([]byte) (int, error) }) (err error) {
	defer func() { recordCustomBuild(siteName, err) }()
	containerfile := ResolveContainerfile(projectPath, cfg)
	if _, err := os.Stat(containerfile); err != nil {
		return fmt.Errorf("containerfile not found: %s", containerfile)
	}

	buildCtx := ResolveBuildContext(projectPath, cfg)
	imageName := CustomImageName(siteName)

	cmd := execCommand(PodmanBin(), buildCustomImageArgs(imageName, containerfile, buildCtx, cfg)...)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building custom image %s: %w", imageName, err)
	}
	return nil
}

// RebuildCustomImage builds and restarts before removing the old image. Removing
// it first with rmi -f killed the running container and spent systemd's restart
// limit on the missing image, leaving the site down.
func RebuildCustomImage(siteName, projectPath string, cfg *config.ContainerConfig, restart func() error) error {
	oldID := LocalImageID(CustomImageName(siteName))
	ForgetCustomImageTools(siteName)
	if err := BuildCustomImage(siteName, projectPath, cfg); err != nil {
		return err
	}
	if err := restart(); err != nil {
		return err
	}
	if oldID != "" && oldID != LocalImageID(CustomImageName(siteName)) {
		_ = execCommand(PodmanBin(), "rmi", oldID).Run()
	}
	return nil
}

// RemoveCustomImage removes the local image for a site's custom container.
func RemoveCustomImage(siteName string) error {
	ForgetCustomImageTools(siteName)
	imageName := CustomImageName(siteName)
	_ = execCommand(PodmanBin(), "rmi", "-f", imageName).Run()
	return nil
}

// ContainerBaseImage reads the Containerfile for a project and returns the
// base image from the first FROM instruction, e.g. "node:20-alpine".
// Returns "" if the file cannot be read or has no FROM line.
func ContainerBaseImage(projectPath string, cfg *config.ContainerConfig) string {
	path := ResolveContainerfile(projectPath, cfg)
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "FROM ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Strip the registry prefix for cleaner display.
				image := parts[1]
				image = strings.TrimPrefix(image, "docker.io/library/")
				image = strings.TrimPrefix(image, "docker.io/")
				return image
			}
		}
	}
	return ""
}

// RemoveCustomContainer removes the stopped container for a site's custom
// container (force-remove to handle edge cases).
func RemoveCustomContainer(siteName string) {
	name := CustomContainerName(siteName)
	_ = execCommand(PodmanBin(), "rm", "-f", name).Run()
}
