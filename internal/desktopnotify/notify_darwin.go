package desktopnotify

import (
	"errors"
	"os/exec"
	"strings"
)

// Supported is false: lerd has no emitter of its own on macOS. The desktop app
// raises notifications there while it listens, and the browser does otherwise.
func Supported() bool { return false }

// Emit is a no-op on macOS.
func Emit(_ Request) (uint32, error) { return 0, errors.New("no native notifier on macOS") }

// schemeHandler is the app LaunchServices opens a scheme's links with, empty
// when there is none. Held in a variable so a test needs no LaunchServices.
var schemeHandler = func(scheme string) string {
	js := `ObjC.import('AppKit'); var u = $.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.URLWithString('` +
		scheme + `://open/')); u.isNil() ? '' : ObjC.unwrap(u.path)`
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", js).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var openURL = func(u string) error { return exec.Command("open", u).Run() }

// AppInstalled reports whether the Lerd desktop app is registered as the lerd://
// handler, so callers can prefer it over a browser.
func AppInstalled() bool { return schemeHandler("lerd") != "" }

// OpenApp focuses (or launches) the desktop app at the given dashboard route.
func OpenApp(route string) error { return openURL(appSchemeURL(route)) }
