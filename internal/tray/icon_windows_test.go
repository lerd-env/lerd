//go:build !nogui && windows

package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

func TestPngToICOWrapsPayload(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 22, 22)))
	pngBytes := buf.Bytes()
	ico := pngToICO(pngBytes)
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 1 {
		t.Fatalf("bad ICONDIR header: % x", ico[:6])
	}
	if !bytes.Equal(ico[22:], pngBytes) {
		t.Fatal("png payload not preserved at offset 22")
	}
	if got := binary.LittleEndian.Uint32(ico[18:]); got != 22 {
		t.Fatalf("image offset = %d, want 22", got)
	}
}

func TestPngToICOKeepsUndecodableInput(t *testing.T) {
	in := []byte("not a png")
	if !bytes.Equal(pngToICO(in), in) {
		t.Fatal("undecodable input should pass through")
	}
}
