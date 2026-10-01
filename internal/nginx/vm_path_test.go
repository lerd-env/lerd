package nginx

import (
	"strings"
	"testing"
)

// The document root is a path inside the container, so it must use forward
// slashes whatever OS lerd runs on.
func TestExpandNginxSnippetJoinsWithForwardSlashes(t *testing.T) {
	out, err := expandNginxSnippet("root {{public}};", "/mnt/c/Sites/app", "public", "lerd-php84-fpm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"/mnt/c/Sites/app/public"`) || strings.Contains(out, `\`) {
		t.Errorf("snippet rendered with a non-POSIX root: %q", out)
	}
}

func TestVhostRootUsesTheContainerPath(t *testing.T) {
	d := VhostData{Path: "/mnt/c/Sites/app", PublicDir: "public"}
	if got := d.Root(); got != `"/mnt/c/Sites/app/public"` {
		t.Errorf("Root() = %q", got)
	}
}
