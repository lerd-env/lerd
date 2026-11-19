// Package fcgi is the FastCGI client lerd's Go web server talks to PHP-FPM
// with; the standard library only ships the responder side.
package fcgi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	typeBeginRequest = 1
	typeEndRequest   = 3
	typeParams       = 4
	typeStdin        = 5
	typeStdout       = 6
	typeStderr       = 7
	roleResponder    = 1
	maxContent       = 65535
	requestID        = 1
)

// Serve sends r to the FastCGI responder at addr and writes its response to w.
// params are added to, and win over, the CGI variables derived from r. An
// error means no response was written, so the caller can still answer 502.
func Serve(w http.ResponseWriter, r *http.Request, addr string, params map[string]string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	body, length, err := requestBody(r)
	if err != nil {
		return err
	}
	env := cgiParams(r, length)
	for k, v := range params {
		env[k] = v
	}

	// The body goes out while the response comes back: a responder that
	// streams, as PHP can, would otherwise fill both buffers and stall.
	go send(conn, env, body) //nolint:errcheck // the response read below reports a dead connection

	out := &stdoutReader{r: bufio.NewReader(conn)}
	if err := writeResponse(w, out); err != nil {
		return err
	}
	// The responder answered, so a body it stopped reading is its choice.
	return nil
}

func requestBody(r *http.Request) (io.Reader, int64, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return bytes.NewReader(nil), 0, nil
	}
	if r.ContentLength >= 0 {
		return r.Body, r.ContentLength, nil
	}
	// A chunked upload has no length, and PHP needs CONTENT_LENGTH up front.
	b, err := io.ReadAll(r.Body)
	return bytes.NewReader(b), int64(len(b)), err
}

func cgiParams(r *http.Request, length int64) map[string]string {
	host, port, _ := net.SplitHostPort(r.RemoteAddr)
	env := map[string]string{
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_SOFTWARE":   "lerd",
		"SERVER_PROTOCOL":   r.Proto,
		"REQUEST_METHOD":    r.Method,
		"REQUEST_URI":       r.URL.RequestURI(),
		"QUERY_STRING":      r.URL.RawQuery,
		"SERVER_NAME":       hostOnly(r.Host),
		"REMOTE_ADDR":       host,
		"REMOTE_PORT":       port,
		"CONTENT_LENGTH":    strconv.FormatInt(length, 10),
		"CONTENT_TYPE":      r.Header.Get("Content-Type"),
		"HTTP_HOST":         r.Host,
	}
	if r.TLS != nil {
		env["HTTPS"] = "on"
	}
	for k, v := range r.Header {
		name := "HTTP_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		if name == "HTTP_PROXY" {
			continue // httpoxy: a client must never set the app's proxy
		}
		env[name] = strings.Join(v, ", ")
	}
	return env
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func send(w io.Writer, env map[string]string, body io.Reader) error {
	begin := []byte{0, roleResponder, 0, 0, 0, 0, 0, 0}
	if err := writeRecord(w, typeBeginRequest, begin); err != nil {
		return err
	}
	var params bytes.Buffer
	for k, v := range env {
		writeLen(&params, len(k))
		writeLen(&params, len(v))
		params.WriteString(k)
		params.WriteString(v)
	}
	if err := writeStream(w, typeParams, &params); err != nil {
		return err
	}
	return writeStream(w, typeStdin, body)
}

// writeStream sends src as records of one type, ended by an empty record.
func writeStream(w io.Writer, typ byte, src io.Reader) error {
	buf := make([]byte, maxContent)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if werr := writeRecord(w, typ, buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return writeRecord(w, typ, nil)
		}
		if err != nil {
			return err
		}
	}
}

func writeRecord(w io.Writer, typ byte, content []byte) error {
	pad := byte((8 - len(content)%8) % 8)
	h := []byte{1, typ, 0, requestID, byte(len(content) >> 8), byte(len(content)), pad, 0}
	_, err := w.Write(append(append(h, content...), make([]byte, pad)...))
	return err
}

func writeLen(b *bytes.Buffer, n int) {
	if n < 128 {
		b.WriteByte(byte(n))
		return
	}
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(n)|1<<31)
	b.Write(l[:])
}

// stdoutReader yields the responder's STDOUT stream and ends at END_REQUEST.
// STDERR is PHP's error log, kept for the caller's logs rather than the page.
type stdoutReader struct {
	r      *bufio.Reader
	buf    []byte
	stderr bytes.Buffer
	done   bool
}

func (s *stdoutReader) Read(p []byte) (int, error) {
	for len(s.buf) == 0 {
		if s.done {
			return 0, io.EOF
		}
		if err := s.next(); err != nil {
			return 0, err
		}
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

func (s *stdoutReader) next() error {
	var h [8]byte
	if _, err := io.ReadFull(s.r, h[:]); err != nil {
		return fmt.Errorf("fastcgi: reading record: %w", err)
	}
	length := int(binary.BigEndian.Uint16(h[4:6]))
	content := make([]byte, length+int(h[6]))
	if _, err := io.ReadFull(s.r, content); err != nil {
		return fmt.Errorf("fastcgi: reading record: %w", err)
	}
	content = content[:length]
	switch h[1] {
	case typeStdout:
		s.buf = content
	case typeStderr:
		s.stderr.Write(content)
	case typeEndRequest:
		s.done = true
	}
	return nil
}

// writeResponse parses the CGI header block and streams the rest as the body.
func writeResponse(w http.ResponseWriter, out *stdoutReader) error {
	br := bufio.NewReader(out)
	hdr, err := textproto.NewReader(br).ReadMIMEHeader()
	if err != nil && !(errors.Is(err, io.EOF) && len(hdr) > 0) {
		return fmt.Errorf("fastcgi: reading response headers: %w", err)
	}
	status := http.StatusOK
	if s := hdr.Get("Status"); s != "" {
		code, convErr := strconv.Atoi(strings.Fields(s)[0])
		if convErr != nil {
			return fmt.Errorf("fastcgi: bad Status %q", s)
		}
		status = code
		hdr.Del("Status")
	} else if hdr.Get("Location") != "" {
		status = http.StatusFound
	}
	for k, v := range hdr {
		w.Header()[k] = v
	}
	w.WriteHeader(status)
	_, err = io.Copy(w, br)
	return err
}
