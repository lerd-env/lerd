package config

import (
	"os"
	"path/filepath"
)

// SetMultiUser turns multi-user mode on or off.
func SetMultiUser(on bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		c.MultiUser = on
		return nil
	})
}

// seatReleasedMarkerPath records that multi-user mode stopped a running stack
// for another user, so it is started again when its user is back. On disk, so
// a restarted watcher still knows; any lerd start, stop or quit by hand clears it.
func seatReleasedMarkerPath() string {
	return filepath.Join(RunDir(), "seat-released")
}

func MarkSeatReleased() error {
	if err := os.MkdirAll(RunDir(), 0755); err != nil {
		return err
	}
	guardRealWrite(seatReleasedMarkerPath())
	return os.WriteFile(seatReleasedMarkerPath(), []byte("released\n"), 0644)
}

func ClearSeatReleased() error {
	if err := os.Remove(seatReleasedMarkerPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func SeatReleased() bool {
	_, err := os.Stat(seatReleasedMarkerPath())
	return err == nil
}
