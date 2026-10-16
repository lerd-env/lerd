package desktopnotify

import (
	"bytes"
	"fmt"
	"html"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/wsl"
)

// ToastAppID is the AppUserModelID lerd's toasts are posted under. Registering
// it in HKCU is what titles them "Lerd" with the logo; no app has to exist.
const ToastAppID = "sh.lerd.Lerd"

// toastLaunchMarker stands in for the click URL, which the script picks on the
// Windows side where it can see which lerd:// handlers are registered.
const toastLaunchMarker = "@LERD_LAUNCH@"

// toastTimeout bounds a PowerShell that hangs, so one stuck toast cannot pile
// up processes in the daemon.
const toastTimeout = 30 * time.Second

// toastXML is the toast content. A route opens on click through protocol
// activation, which Windows handles itself, so nothing has to listen for it.
func toastXML(req Request) string {
	attrs := ""
	if req.Route != "" {
		attrs += ` launch="` + toastLaunchMarker + `" activationType="protocol"`
	}
	if req.Urgency == UrgencyCritical {
		attrs += ` duration="long"`
	}
	audio := ""
	if req.Urgency == UrgencyLow {
		audio = `<audio silent="true"/>`
	}
	return `<toast` + attrs + `><visual><binding template="ToastGeneric"><text>` + html.EscapeString(req.Summary) +
		`</text><text>` + html.EscapeString(req.Body) + `</text></binding></visual>` + audio + `</toast>`
}

// psQuote is s as a single-quoted PowerShell literal.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// toastScript registers the app id with the bundled icon, copied from the
// distro on first use, and shows the toast. The click goes to the desktop app,
// then an installed PWA, then the dashboard, as on the D-Bus path.
func toastScript(req Request, iconSrc string) string {
	esc := func(u string) string { return psQuote(html.EscapeString(u)) }
	return `$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$id = ` + psQuote(ToastAppID) + `
$icon = Join-Path $env:LOCALAPPDATA 'lerd\notify-icon.png'
$src = ` + psQuote(iconSrc) + `
if ($src -and -not (Test-Path -LiteralPath $icon) -and (Test-Path -LiteralPath $src)) {
	New-Item -ItemType Directory -Force (Split-Path $icon) | Out-Null
	Copy-Item -LiteralPath $src $icon
}
$k = "HKCU:\Software\Classes\AppUserModelId\$id"
if (-not (Test-Path $k)) { New-Item -Force $k | Out-Null }
Set-ItemProperty $k DisplayName 'Lerd'
if (Test-Path -LiteralPath $icon) { Set-ItemProperty $k IconUri $icon }
$u = if (Test-Path 'HKCU:\Software\Classes\lerd') { ` + esc(appSchemeURL(req.Route)) + ` }
	elseif (Test-Path 'HKCU:\Software\Classes\web+lerd') { ` + esc(pwaSchemeURL(req.Route)) + ` }
	else { ` + esc(browserURL(req.Route)) + ` }
$xml = ` + psQuote(toastXML(req)) + `.Replace(` + psQuote(toastLaunchMarker) + `, $u)
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
[void][Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
$d = New-Object Windows.Data.Xml.Dom.XmlDocument
$d.LoadXml($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($d))
`
}

// toastSupported reports whether Windows PowerShell is reachable over interop.
func toastSupported() bool {
	return wsl.PowerShell() != ""
}

// emitToast shows req as a Windows toast. It returns once PowerShell starts;
// the toast takes a few hundred milliseconds and is not worth holding a caller.
func emitToast(req Request) error {
	ps := wsl.PowerShell()
	if ps == "" {
		return fmt.Errorf("powershell.exe not reachable from WSL")
	}
	iconSrc := ""
	if p := IconPath(); p != "" {
		if w, err := exec.Command("wslpath", "-w", p).Output(); err == nil {
			iconSrc = strings.TrimSpace(string(w))
		}
	}
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-EncodedCommand", wsl.EncodePowerShell(toastScript(req, iconSrc)))
	// A Windows cwd: from a distro path Windows tools warn about UNC paths.
	cmd.Dir = filepath.Dir(ps)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		t := time.AfterFunc(toastTimeout, func() { _ = cmd.Process.Kill() })
		defer t.Stop()
		if err := cmd.Wait(); err != nil {
			fmt.Printf("[notifier] windows toast failed: %v %s\n", err, out.String())
		}
	}()
	return nil
}

// windowsSchemeRegistered reports whether a Windows app claims scheme, as the
// Lerd desktop app does for lerd://.
func windowsSchemeRegistered(scheme string) bool {
	return exec.Command(wsl.WindowsTool("reg.exe"), "query", `HKCU\Software\Classes\`+scheme).Run() == nil
}
