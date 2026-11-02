package nginx

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ReloadOrWarn reloads nginx and prints one warning line if that fails, with
// indent as its prefix. A stopped nginx is not a failure, since it reads the
// vhosts when it starts.
func ReloadOrWarn(indent string) {
	reloadOrWarn(Reload, os.Stdout, indent)
}

func reloadOrWarn(reload func() error, w io.Writer, indent string) {
	if err := reload(); err != nil && !errors.Is(err, ErrNotRunning) {
		fmt.Fprintf(w, "%s[WARN] nginx reload: %v\n", indent, err)
	}
}
