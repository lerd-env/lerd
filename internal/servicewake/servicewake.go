// Package servicewake is how a lerd process asks the watcher to bring a service
// up and waits for the answer, so the watcher's one manager starts and probes
// each service once however many processes (CLI, MCP, a site's link) ask.
package servicewake

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// A request is one line, "ensure <name>". The watcher answers "starting" first
// when the service is not ready yet, then "ok", or "error" and the reason up to
// the end of the stream.
const (
	replyStarting = "starting"
	replyOK       = "ok"
	replyError    = "error"
)

// askWait bounds the whole answer: a first start can pull an image.
var askWait = 10 * time.Minute

// Ask asks the watcher to bring name up and waits. handled is false when no
// watcher answers (stopped or restarting) and the caller does it itself;
// onStarting runs when the watcher says it has to start the service.
func Ask(name string, onStarting func()) (handled bool, err error) {
	// A name with a newline in it would send a second request of its own.
	if !config.IsValidServiceName(name) {
		return true, errors.New("not a service name: " + strconv.Quote(name))
	}
	conn, err := net.DialTimeout("unix", config.ServiceWakeSocketPath(), time.Second)
	if err != nil {
		return false, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(askWait))
	if _, err := io.WriteString(conn, "ensure "+name+"\n"); err != nil {
		return false, nil
	}
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return false, nil // dropped before answering: do it here instead
		}
		switch strings.TrimSpace(line) {
		case replyStarting:
			if onStarting != nil {
				onStarting()
			}
		case replyOK:
			return true, nil
		case replyError:
			reason, _ := io.ReadAll(r)
			return true, errors.New(string(reason))
		}
	}
}

// Listen binds the watcher's socket, readable by this user only.
func Listen() (net.Listener, error) {
	path := config.ServiceWakeSocketPath()
	if err := os.MkdirAll(config.RunDir(), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Serve answers requests on l until it closes, each on its own goroutine:
// ensure brings the service up and returns once it is ready, and ready says
// whether it already is, so a caller hears "starting" only for a real start.
func Serve(l net.Listener, ready func(string) bool, ensure func(string) error) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go answer(conn, ready, ensure)
	}
}

func answer(conn net.Conn, ready func(string) bool, ensure func(string) error) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(io.LimitReader(conn, 256)).ReadString('\n')
	if err != nil {
		return
	}
	verb, name, _ := strings.Cut(strings.TrimRight(line, "\n"), " ")
	if verb != "ensure" || !config.IsValidServiceName(name) {
		_, _ = io.WriteString(conn, replyError+"\nnot a request for a service: "+strings.TrimSpace(line))
		return
	}
	_ = conn.SetDeadline(time.Time{})
	if !ready(name) {
		_, _ = io.WriteString(conn, replyStarting+"\n")
	}
	if err := ensure(name); err != nil {
		_, _ = io.WriteString(conn, replyError+"\n"+err.Error())
		return
	}
	_, _ = io.WriteString(conn, replyOK+"\n")
}
