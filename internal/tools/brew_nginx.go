package tools

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
)

// Seams for the two binaries the Homebrew nginx row reads, nginx and brew.
var (
	brewLook = hostbin.Look
	brewRun  = func(bin string, args ...string) ([]byte, error) {
		return exec.Command(bin, args...).CombinedOutput()
	}
)

// hostNginxStatus is the Homebrew nginx lerd installs when PHP moves to the
// host and runs nginx on. Homebrew owns its versions, so its pin is the version
// `brew upgrade nginx` would bring, read from brew's local index.
func hostNginxStatus() (ToolStatus, bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.NginxRuntimeMode() != config.NginxRuntimeHost {
		return ToolStatus{}, false
	}
	bin, ok := brewLook("nginx")
	if !ok {
		return ToolStatus{}, false
	}
	s := ToolStatus{Name: "nginx", Present: true}
	if out, err := brewRun(bin, "-v"); err == nil {
		_, s.Installed, _ = strings.Cut(strings.TrimSpace(string(out)), "nginx/")
	}
	s.Pinned = brewStable("nginx")
	s.UpdateAvailable = s.Installed != "" && s.Pinned != "" && !sameVersion(s.Installed, s.Pinned)
	return s, true
}

// brewStable is the stable version Homebrew's index carries for formula.
func brewStable(formula string) string {
	brew, ok := brewLook("brew")
	if !ok {
		return ""
	}
	out, err := brewRun(brew, "info", "--json=v2", formula)
	if err != nil {
		return ""
	}
	var info struct {
		Formulae []struct {
			Versions struct{ Stable string } `json:"versions"`
		} `json:"formulae"`
	}
	if json.Unmarshal(out, &info) != nil || len(info.Formulae) == 0 {
		return ""
	}
	return info.Formulae[0].Versions.Stable
}

// podmanStatus lists the podman lerd runs everything on, for its version only:
// an upgrade can change the machine under every container, so it stays the
// user's to make.
func podmanStatus() (ToolStatus, bool) {
	bin, ok := brewLook("podman")
	if !ok {
		return ToolStatus{}, false
	}
	s := ToolStatus{Name: "podman", Present: true, External: true}
	if out, err := brewRun(bin, "--version"); err == nil {
		_, s.Installed, _ = strings.Cut(strings.TrimSpace(string(out)), "version ")
	}
	return s, true
}
