package cli

import (
	"fmt"
	"io"
)

// fetchMise saves the bare mise.exe Windows releases ship to dest, verified,
// returning the installed version.
func fetchMise(pins *pinnedTools, dest string, w io.Writer) (string, error) {
	v, err := pins.download("mise", dest, 0755, w)
	if err != nil {
		return "", fmt.Errorf("mise download: %w", err)
	}
	return v, nil
}
