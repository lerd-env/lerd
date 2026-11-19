package podman

import "sync"

var (
	backgroundReadMu sync.Mutex
	onBackgroundRead func(unit string)
)

// SetOnBackgroundRead registers what runs when a background read (a digest
// fetch, a PHP patch probe) finishes with a new answer about unit, so lerd-ui
// can drop what it cached for that unit and rebuild.
func SetOnBackgroundRead(fn func(unit string)) {
	backgroundReadMu.Lock()
	onBackgroundRead = fn
	backgroundReadMu.Unlock()
}

func announceBackgroundRead(unit string) {
	backgroundReadMu.Lock()
	fn := onBackgroundRead
	backgroundReadMu.Unlock()
	if fn != nil {
		fn(unit)
	}
}
