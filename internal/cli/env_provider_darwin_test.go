//go:build darwin

package cli

import (
	"bytes"
	"testing"
)

func stubProvidedEnvSSH(t *testing.T) *[]string {
	var scripts []string
	orig := providedEnvSSH
	providedEnvSSH = func(script string, _ []byte, _ *bytes.Buffer) error {
		scripts = append(scripts, script)
		return nil
	}
	t.Cleanup(func() { providedEnvSSH = orig; providedEnvListing.set(nil) })
	return &scripts
}

// Within a pass, a site the listing does not show costs no ssh.
func TestDropProvidedEnvVM_skipsAFileThePassDidNotList(t *testing.T) {
	scripts := stubProvidedEnvSSH(t)
	providedEnvListing.set(map[string]bool{"other.env": true})

	dropProvidedEnvVM("app")

	if len(*scripts) != 0 {
		t.Errorf("ran %v for a file the pass did not list", *scripts)
	}
}

// lerd-ui and the watcher live for days, so outside a pass a file another
// process wrote since must still be removed.
func TestDropProvidedEnvVM_alwaysRemovesOutsideAPass(t *testing.T) {
	scripts := stubProvidedEnvSSH(t)

	dropProvidedEnvVM("app")

	if len(*scripts) != 1 || (*scripts)[0] != providedEnvRemoveScript("app") {
		t.Errorf("scripts = %v, want the remove script", *scripts)
	}
}
