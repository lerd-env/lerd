//go:build !nogui

package tray

import _ "embed"

// The PNGs are rendered from the SVGs next to them by assets/generate.sh.

//go:embed assets/icon-white.png
var iconWhitePNG []byte

//go:embed assets/icon-dark.png
var iconDarkPNG []byte

//go:embed assets/icon-green.png
var iconGreenPNG []byte

//go:embed assets/icon-off-white.png
var iconOffWhitePNG []byte

//go:embed assets/icon-off-dark.png
var iconOffDarkPNG []byte

//go:embed assets/icon-off-red.png
var iconOffRedPNG []byte

//go:embed assets/icon-mono.png
var iconMonoPNG []byte

//go:embed assets/icon-mono-off.png
var iconMonoOffPNG []byte
