package cli

import (
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// hotkeyPollInterval is how long, in milliseconds, a poll waits before the
// reader re-checks whether it was asked to stop.
const hotkeyPollInterval = 50

// hotkeyReader watches the terminal for the two control keys a progress view
// acts on, Ctrl+O and Ctrl+C, and can be called off. It holds nothing: each byte
// goes straight to handle, which compares it against those two and drops
// everything else.
//
// It polls before every read instead of parking in a blocking one, because a
// goroutine already blocked reading a terminal cannot be stopped: closing the
// descriptor leaves that read waiting. Such a reader outlives the view that
// started it and goes on competing for typed bytes, so the next prompt loses
// its answer, or the newline ending it, and waits forever.
type hotkeyReader struct {
	quit chan struct{}
	done chan struct{}
	once sync.Once
}

// startHotkeys duplicates src and passes each byte read from it to handle,
// until stop is called or the descriptor ends. Returns nil, which stop accepts,
// when the descriptor cannot be duplicated or cannot be polled.
//
// The poll check is not optional. On macOS poll(2) does not support devices,
// and /dev/tty, which the installer script hands lerd as stdin, is one: poll
// returns POLLNVAL at once, and a reader that took that for input parked in a
// read only a full line could end, while the view's stop waited on it. That is
// the install standing still after the PHP images until someone pressed Enter.
// A descriptor poll cannot watch gets no reader at all, and the view stays in
// cooked mode so Ctrl+C still reaches it as a signal.
func startHotkeys(src int, handle func(b byte)) *hotkeyReader {
	fd, err := syscall.Dup(src)
	if err != nil {
		return nil
	}
	if !pollable(fd) {
		syscall.Close(fd) //nolint:errcheck
		return nil
	}
	k := &hotkeyReader{quit: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(k.done)
		defer syscall.Close(fd) //nolint:errcheck
		buf := make([]byte, 1)
		for {
			select {
			case <-k.quit:
				return
			default:
			}
			pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(pfd, hotkeyPollInterval)
			if err != nil {
				if err == unix.EINTR {
					continue
				}
				return
			}
			if n == 0 {
				continue
			}
			// n counts descriptors with any event, not only input. Reading on
			// POLLNVAL or POLLHUP would park in a read nothing ends.
			if pfd[0].Revents&unix.POLLNVAL != 0 {
				return
			}
			if pfd[0].Revents&unix.POLLIN == 0 {
				continue
			}
			read, err := unix.Read(fd, buf)
			if read > 0 {
				handle(buf[0])
			}
			if read == 0 || (err != nil && err != unix.EINTR) {
				return
			}
		}
	}()
	return k
}

// pollable reports whether poll(2) can watch fd. A zero timeout answers at
// once: a descriptor poll rejects comes back with POLLNVAL instead of waiting.
func pollable(fd int) bool {
	pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(pfd, 0)
	if err != nil && err != unix.EINTR {
		return false
	}
	return n == 0 || pfd[0].Revents&unix.POLLNVAL == 0
}

// stop ends the reader and returns only once its goroutine is gone, so the
// caller knows nothing of its own is still reading the terminal.
func (k *hotkeyReader) stop() {
	if k == nil {
		return
	}
	k.once.Do(func() { close(k.quit) })
	<-k.done
}
