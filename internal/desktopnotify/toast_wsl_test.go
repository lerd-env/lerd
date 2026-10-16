package desktopnotify

import (
	"strings"
	"testing"
)

func TestToastXMLEscapesText(t *testing.T) {
	x := toastXML(Request{Summary: `Mail <from> "a" & b`, Body: "it's > 1"})
	for _, want := range []string{`Mail &lt;from&gt; &#34;a&#34; &amp; b`, `it&#39;s &gt; 1`} {
		if !strings.Contains(x, want) {
			t.Errorf("toast XML %q lacks %q", x, want)
		}
	}
}

// A route makes the toast open a URL on click; without one it only dismisses.
func TestToastXMLLaunchOnlyWithRoute(t *testing.T) {
	if x := toastXML(Request{Summary: "s"}); strings.Contains(x, "launch=") {
		t.Errorf("toast without a route must not launch anything: %s", x)
	}
	x := toastXML(Request{Summary: "s", Route: "#mail"})
	if !strings.Contains(x, `launch="`+toastLaunchMarker+`" activationType="protocol"`) {
		t.Errorf("toast with a route must launch the chosen URL: %s", x)
	}
}

func TestToastXMLUrgency(t *testing.T) {
	if x := toastXML(Request{Urgency: UrgencyCritical}); !strings.Contains(x, `duration="long"`) {
		t.Errorf("a critical toast must stay up longer: %s", x)
	}
	if x := toastXML(Request{Urgency: UrgencyLow}); !strings.Contains(x, `<audio silent="true"/>`) {
		t.Errorf("a low urgency toast must be silent: %s", x)
	}
	if x := toastXML(Request{Urgency: UrgencyNormal}); strings.Contains(x, "duration=") || strings.Contains(x, "<audio") {
		t.Errorf("a normal toast keeps the Windows defaults: %s", x)
	}
}

// The click goes to the desktop app first, then an installed PWA, then the
// browser, the same order the D-Bus path follows.
func TestToastScriptPicksClickTarget(t *testing.T) {
	s := toastScript(Request{Summary: "s", Route: "#mail"}, `\\wsl.localhost\Ubuntu\home\o'brien\icon.png`)
	app := strings.Index(s, `'lerd://open/#mail'`)
	pwa := strings.Index(s, `'web+lerd://open/#mail'`)
	web := strings.Index(s, `'http://lerd.localhost/#mail'`)
	if app < 0 || pwa < 0 || web < 0 || !(app < pwa && pwa < web) {
		t.Errorf("click targets missing or out of order in:\n%s", s)
	}
	if !strings.Contains(s, `'\\wsl.localhost\Ubuntu\home\o''brien\icon.png'`) {
		t.Errorf("single quotes in the icon path must be doubled:\n%s", s)
	}
	if !strings.Contains(s, `'`+ToastAppID+`'`) {
		t.Errorf("script must post under the lerd app id:\n%s", s)
	}
}

func TestToastURLEscapedForXML(t *testing.T) {
	s := toastScript(Request{Summary: "s", Route: "#sites?a=1&b=2"}, "")
	if !strings.Contains(s, `'http://lerd.localhost/#sites?a=1&amp;b=2'`) {
		t.Errorf("the launch URL lands in an XML attribute and must be escaped:\n%s", s)
	}
}
