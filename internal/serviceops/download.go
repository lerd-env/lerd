package serviceops

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// RetagImage swaps an image reference's tag, keeping its repository. Shared by
// the update path and its estimate so a chosen tag resolves the same way twice.
func RetagImage(image, tag string) string {
	if image == "" || tag == "" {
		return image
	}
	if at := strings.LastIndex(image, ":"); at > 0 && !strings.Contains(image[at:], "/") {
		return image[:at] + ":" + tag
	}
	return image + ":" + tag
}

// PresetDownload reports the image installing preset name at version would
// pull, without installing anything.
func PresetDownload(name, version string) (podman.PendingDownload, error) {
	preset, err := config.EnsurePreset(name)
	if err != nil {
		return podman.PendingDownload{}, err
	}
	svc, err := preset.Resolve(version)
	if err != nil {
		return podman.PendingDownload{}, err
	}
	return podman.DescribeDownload(svc.Image), nil
}

// ActionDownload reports the image a service operation would pull. An action
// with nothing to fetch (an update that is already current) returns a zero
// value, which callers read as "no download, go ahead".
func ActionDownload(name, action, tag string) (podman.PendingDownload, error) {
	switch action {
	case "update", "migrate":
		avail, err := CheckUpdateAvailable(name)
		if err != nil || avail == nil {
			return podman.PendingDownload{}, err
		}
		if tag != "" {
			if action == "update" {
				return podman.DescribeDownload(RetagImage(avail.CurrentImage, tag)), nil
			}
			target, err := ResolveMigrateTarget(name, avail.CurrentImage, tag)
			if err != nil {
				return podman.PendingDownload{}, err
			}
			return podman.DescribeDownload(target), nil
		}
		if !avail.Available {
			return podman.PendingDownload{}, nil
		}
		return podman.DescribeDownload(avail.LatestImage), nil
	case "rollback":
		_, previous := serviceImageRefs(name)
		return podman.DescribeDownload(previous), nil
	case "start":
		return podman.DescribeDownload(StartImage(name)), nil
	case "add":
		// Adding a suggested service to a site may install a store preset
		// first, one this machine may not even have fetched yet.
		if img := StartImage(name); img != "" {
			return podman.DescribeDownload(img), nil
		}
		if d, err := PresetDownload(name, ""); err == nil {
			return d, nil
		}
		return podman.PendingDownload{}, nil
	case "reinstall":
		current, _ := serviceImageRefs(name)
		return podman.DescribeDownload(current), nil
	}
	return podman.PendingDownload{}, fmt.Errorf("unknown action %q", action)
}
