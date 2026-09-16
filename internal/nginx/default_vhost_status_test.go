package nginx

import (
	"strings"
	"testing"
)

// An unknown or just-unlinked domain lands on the catch-all. try_files serves
// the page it finds with a 200, and its =404 fallback only fires when the file
// is missing, so "Site Not Found" answered 200 and an unlinked site looked like
// it was still being served.
func TestDefaultVhostAnswersNotFound(t *testing.T) {
	conf := string(renderDefaultVhost())

	if strings.Contains(conf, "try_files /404.html") {
		t.Errorf("try_files serves the page with 200:\n%s", conf)
	}
	if !strings.Contains(conf, "return 404;") {
		t.Errorf("the catch-all must answer 404:\n%s", conf)
	}
	if !strings.Contains(conf, "error_page 404 /404.html;") {
		t.Errorf("the 404 must still render the lerd page:\n%s", conf)
	}
	if !strings.Contains(conf, "internal;") {
		t.Errorf("the error page location must not be requestable directly:\n%s", conf)
	}
}
