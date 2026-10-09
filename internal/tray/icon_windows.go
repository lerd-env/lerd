//go:build !nogui && windows

package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/png"
)

// The Windows shell loads tray icons as .ico, not PNG. An ico may carry a PNG
// payload verbatim (Vista and later), so each embedded PNG is wrapped in a
// one-image ico container at startup.
func init() {
	for _, p := range []*[]byte{&iconPNG, &iconWhitePNG, &iconDarkPNG, &iconGreenPNG, &iconMonoPNG} {
		*p = pngToICO(*p)
	}
}

func pngToICO(png []byte) []byte {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		return png
	}
	dim := func(n int) byte {
		if n >= 256 {
			return 0
		}
		return byte(n)
	}
	var b bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), one image.
	_ = binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, 1})
	// ICONDIRENTRY: width, height, colours, reserved, planes, bpp, size, offset.
	b.Write([]byte{dim(cfg.Width), dim(cfg.Height), 0, 0})
	_ = binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
	_ = binary.Write(&b, binary.LittleEndian, [2]uint32{uint32(len(png)), 22})
	b.Write(png)
	return b.Bytes()
}
