package ui

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// A client declares the payload length before any data arrives, so a forged
// 64-bit length must be rejected before ReadFrame allocates — otherwise one
// frame forces a multi-gigabyte allocation on /api/ws and /api/lsp/php.
func TestReadFrame_RejectsOversizedDeclaredLength(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(wsOpText)   // fin bit unset is fine, opcode is all ReadFrame inspects
	buf.WriteByte(0x80 | 127) // masked, 64-bit extended length follows
	var ext [8]byte
	binary.BigEndian.PutUint64(ext[:], uint64(maxWSFrameBytes)+1)
	buf.Write(ext[:])

	c := &wsConn{br: bufio.NewReader(&buf)}
	_, payload, err := c.ReadFrame()
	if !errors.Is(err, errFrameTooLarge) {
		t.Fatalf("expected errFrameTooLarge, got %v", err)
	}
	if payload != nil {
		t.Fatalf("expected no payload allocation, got %d bytes", len(payload))
	}
}
