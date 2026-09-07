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
// when the descriptor cannot be duplicated.
func startHotkeys(src int, handle func(b byte)) *hotkeyReader {
	fd, err := syscall.Dup(src)
	if err != nil {
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
			n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, hotkeyPollInterval)
			if err != nil {
				if err == unix.EINTR {
					continue
				}
				return
			}
			if n == 0 {
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

// stop ends the reader and returns only once its goroutine is gone, so the
// caller knows nothing of its own is still reading the terminal.
func (k *hotkeyReader) stop() {
	if k == nil {
		return
	}
	k.once.Do(func() { close(k.quit) })
	<-k.done
}
