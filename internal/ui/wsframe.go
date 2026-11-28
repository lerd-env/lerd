package ui

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Hand-rolled RFC6455 helpers, kept in-tree rather than a dependency: handshake,
// unmasked text and binary frames out (JSON, a plugin terminal's raw bytes),
// masked frames in, close and ping/pong. No fragmentation, no compression.

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Opcodes we care about.
const (
	wsOpText   = 0x1
	wsOpBinary = 0x2
	wsOpClose  = 0x8
	wsOpPing   = 0x9
	wsOpPong   = 0xA
)

// maxWSFrameBytes caps a single inbound frame payload: the client declares the
// length before any data arrives, so without a ceiling a forged length forces a
// multi-gigabyte allocation. 64 MiB matches the LSP message ceiling.
const maxWSFrameBytes = 64 << 20

// errFrameTooLarge is returned when a client declares a payload over the cap.
var errFrameTooLarge = errors.New("ui: websocket frame exceeds size limit")

// wsConn wraps a hijacked net.Conn with the buffered reader from the
// handshake so WriteText and ReadFrame can share the same underlying stream.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
}

// wsOriginAllowed defends against cross-site WebSocket hijacking: browsers
// don't apply CORS to the WS handshake, so without this an arbitrary page
// could open a socket to the dashboard. A missing Origin means a non-browser
// client (lerd's own tools, curl), which isn't a hijack vector. Otherwise the
// Origin must be in the CORS allowlist, or be same-origin with the request Host
// AND target a local-network host, which keeps custom-domain and LAN access
// working while closing the DNS-rebinding hole (see isLocalNetworkHost).
func wsOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if allowedCORSOrigins[origin] {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// Hostnames are case-insensitive (RFC 3986), and r.Host casing isn't
	// guaranteed to match the browser-normalised Origin, so compare case-folded.
	if !strings.EqualFold(u.Host, r.Host) {
		return false
	}
	return isLocalNetworkHost(u.Host)
}

// isLocalNetworkHost reports whether hostport (host or host:port) names a
// machine on the local network. The same-origin check above is not enough on
// its own: under DNS rebinding a page on http://evil.example whose A record is
// rebound to a loopback or LAN address sends Origin and Host both equal to
// evil.example, so a bare same-origin test passes and the socket opens.
//
// Rebinding fundamentally needs a DNS name (an IP-literal Host can't be
// rebound), so the rule is: any IP literal is fine (it's a host the user
// pointed a browser at directly: loopback, a LAN address, or an explicit
// public bind that is already gated by remote-control auth), while a hostname
// is accepted only under localhost or one of the reserved local TLDs. .test and
// .localhost are lerd's own managed TLDs (resolved to loopback by lerd-dns,
// never publicly resolvable per RFC 6761) and .local is mDNS, so none can be
// pointed at an attacker's server. An attacker's public domain is rejected.
func isLocalNetworkHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]") // strip IPv6 literal brackets
	if net.ParseIP(host) != nil {
		return true
	}
	h := strings.ToLower(host)
	return h == "localhost" ||
		strings.HasSuffix(h, ".localhost") ||
		strings.HasSuffix(h, ".test") ||
		strings.HasSuffix(h, ".local")
}

// wsUpgrade performs the RFC6455 handshake and returns a wsConn ready for
// ReadFrame / WriteText. The http.ResponseWriter must support Hijack.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !wsOriginAllowed(r) {
		return nil, errors.New("websocket origin not allowed")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("missing Upgrade: websocket header")
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, errors.New("missing Connection: upgrade header")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijack")
	}
	conn, brw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := brw.Writer.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := brw.Writer.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, br: brw.Reader}, nil
}

// WriteText sends a single unfragmented text frame.
func (c *wsConn) WriteText(payload []byte) error {
	return c.writeFrame(wsOpText, payload)
}

// WriteBinary sends a single unfragmented binary frame, for bytes that need
// not be valid UTF-8, such as a terminal's output cut mid-character.
func (c *wsConn) WriteBinary(payload []byte) error {
	return c.writeFrame(wsOpBinary, payload)
}

// WritePing sends a ping frame so the server can probe whether a silent client
// is still reachable. Browsers reply with a pong, which arrives on the read
// path and refreshes the read deadline.
func (c *wsConn) WritePing(payload []byte) error {
	return c.writeFrame(wsOpPing, payload)
}

// WritePong sends a pong frame in reply to a ping, echoing the payload.
func (c *wsConn) WritePong(payload []byte) error {
	return c.writeFrame(wsOpPong, payload)
}

// SetReadDeadline forwards to the underlying connection so callers can bound
// how long ReadFrame blocks waiting for the next frame.
func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// WriteClose sends a close frame with no payload.
func (c *wsConn) WriteClose() error {
	return c.writeFrame(wsOpClose, nil)
}

func (c *wsConn) writeFrame(op byte, payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	header := make([]byte, 2, 10)
	header[0] = 0x80 | op // FIN | opcode
	n := len(payload)
	switch {
	case n < 126:
		header[1] = byte(n)
	case n <= 0xFFFF:
		header[1] = 126
		header = binary.BigEndian.AppendUint16(header, uint16(n))
	default:
		header[1] = 127
		header = binary.BigEndian.AppendUint64(header, uint64(n))
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Close shuts down the underlying connection.
func (c *wsConn) Close() error { return c.conn.Close() }

// ReadFrame reads one frame and returns its opcode and payload. Client frames
// are always masked per RFC6455; the mask is applied before return.
func (c *wsConn) ReadFrame() (op byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(c.br, hdr[:]); err != nil {
		return
	}
	op = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	plen := int(hdr[1] & 0x7F)
	switch plen {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		plen = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		plen = int(binary.BigEndian.Uint64(ext[:]))
	}
	if plen < 0 || plen > maxWSFrameBytes {
		err = errFrameTooLarge
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, plen)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}
