//go:build !nogui && windows

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows"
)

// Win32 menus draw emoji in monochrome, so the coloured dots would all come out
// grey. Windows items get the dot as a real menu icon instead.
var dotColors = map[string]color.NRGBA{
	"🟢": {0x2E, 0xCC, 0x40, 0xFF},
	"🔴": {0xE7, 0x4C, 0x3C, 0xFF},
	"🟡": {0xF1, 0xC4, 0x0F, 0xFF},
	"⚪": {0xB0, 0xB0, 0xB0, 0xFF},
}

var (
	dotMu    sync.Mutex
	dotIcons = map[string][]byte{}
	hasDot   = map[*systray.MenuItem]bool{}
)

func dotIcon(dot string) []byte {
	if b, ok := dotIcons[dot]; ok {
		return b
	}
	const size = 16
	// The shell paints menu bitmaps without alpha, so a transparent corner turns
	// black. Draw onto the menu colour instead.
	bg := menuColor()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, 0xFF
	}
	c, ok := dotColors[dot]
	if dot != "" && ok {
		const r = 5.5
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx, dy := float64(x)+0.5-size/2, float64(y)+0.5-size/2
				d := dx*dx + dy*dy
				switch {
				case d <= (r-0.7)*(r-0.7):
					img.SetNRGBA(x, y, c)
				case d <= (r+0.7)*(r+0.7):
					img.SetNRGBA(x, y, color.NRGBA{(c.R + bg.R) / 2, (c.G + bg.G) / 2, (c.B + bg.B) / 2, 0xFF})
				}
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	b := pngToICO(buf.Bytes())
	dotIcons[dot] = b
	return b
}

// setItemTitle sets a menu title, lifting a leading status dot into a coloured
// menu icon. An item that had a dot and no longer does gets a blank icon back.
func setItemTitle(item *systray.MenuItem, title string) {
	dot, rest := splitDot(title)
	dotMu.Lock()
	defer dotMu.Unlock()
	switch {
	case dot != "":
		item.SetTitle(rest)
		item.SetIcon(dotIcon(dot))
		hasDot[item] = true
	case hasDot[item]:
		item.SetTitle(title)
		item.SetIcon(dotIcon(""))
		hasDot[item] = false
	default:
		item.SetTitle(title)
	}
}

var procGetSysColor = windows.NewLazySystemDLL("user32.dll").NewProc("GetSysColor")

// menuColor is the system menu background (COLOR_MENU), a COLORREF of 0x00BBGGRR.
func menuColor() color.NRGBA {
	const colorMenu = 4
	v, _, _ := procGetSysColor.Call(colorMenu)
	return color.NRGBA{R: byte(v), G: byte(v >> 8), B: byte(v >> 16), A: 0xFF}
}

// disableInfoItem leaves a read-only status row enabled: Win32 greys the icon of
// a disabled item, which would turn the status dot grey. Clicks do nothing.
func disableInfoItem(*systray.MenuItem) {}
