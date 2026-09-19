package podman

import (
	"strings"
	"testing"
)

// The quadlets are the reference. Every long-running lerd container opts out of
// SELinux labelling, which is why none of them trips over a bind-mounted host
// path, and a one-off run has to say the same thing spelled the same way.
func TestHostMountRunArgsMatchTheQuadlets(t *testing.T) {
	joined := strings.Join(HostMountRunArgs(), " ")
	if !strings.Contains(joined, "label=disable") {
		t.Fatalf("HostMountRunArgs() = %q, want the SELinux opt-out", joined)
	}
	tmpl, err := GetQuadletTemplate("lerd-php-fpm.container.tmpl")
	if err != nil {
		t.Fatalf("read quadlet template: %v", err)
	}
	if !strings.Contains(tmpl, "label=disable") {
		t.Error("the quadlet no longer disables labelling, so these two have drifted apart")
	}
}
