package cli

import (
	"strings"
	"testing"
)

// A removed folder still named in a PHP quadlet keeps the container from
// starting, so doctor has to name it rather than stay silent while every site
// answers 502.
func TestPHPMountsFinding(t *testing.T) {
	if _, bad := phpMountsFinding(nil); bad {
		t.Error("no stale mounts is not a finding")
	}
	msg, bad := phpMountsFinding([]string{"/srv/shop/shop-feat"})
	if !bad || !strings.Contains(msg, "/srv/shop/shop-feat") {
		t.Errorf("phpMountsFinding = %q, %v, want a failure naming the folder", msg, bad)
	}
}
