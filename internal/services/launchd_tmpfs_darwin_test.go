package services

import (
	"strings"
	"testing"
)

// The macOS path runs containers through podman run rather than systemd, so a
// Tmpfs= key in the quadlet has to be translated here or it is silently dropped
// and the cache it was meant to hold stays on the bind mount.
func TestContainerToPodmanArgs_TranslatesTmpfs(t *testing.T) {
	args, err := containerToPodmanArgs(map[string][]string{
		"Image":  {"lerd-php85-fpm:local"},
		"Volume": {"/Users/u:/Users/u:rw"},
		"Tmpfs":  {"/Users/u/site/var/cache"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--tmpfs /Users/u/site/var/cache") {
		t.Errorf("tmpfs not translated: %s", joined)
	}
	if strings.Index(joined, "--tmpfs") < strings.Index(joined, "-v /Users/u:/Users/u") {
		t.Errorf("the tmpfs must come after the bind mount it shadows: %s", joined)
	}
}

func TestContainerToPodmanArgs_ExpandsSpecifiersInTmpfs(t *testing.T) {
	args, err := containerToPodmanArgs(map[string][]string{
		"Image": {"x:local"},
		"Tmpfs": {"%h/site/var/cache"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); strings.Contains(joined, "%h") {
		t.Errorf("%%h left unexpanded: %s", joined)
	}
}
