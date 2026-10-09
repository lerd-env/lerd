//go:build windows

package cli

// hotkeyReader has no Windows implementation yet: the progress view relies on
// polling a duplicated terminal descriptor, which Windows consoles do not offer.
// startHotkeys returns nil, which stop accepts, so the view simply runs without
// the Ctrl+O toggle and Ctrl+C keeps reaching it as an interrupt.
type hotkeyReader struct{}

func startHotkeys(_ int, _ func(b byte)) *hotkeyReader { return nil }

func (k *hotkeyReader) stop() {}
