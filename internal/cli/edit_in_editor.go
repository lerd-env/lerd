package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// editInEditor opens body in $EDITOR through a temp file and returns the edit.
// ok is false when there is nothing to save: no editor launched (the real path
// is printed instead) or the text came back unchanged.
func editInEditor(cmd *cobra.Command, body, pattern, realPath string) (edited string, ok bool, err error) {
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	launched, err := launchEditor(tmpPath)
	if err != nil {
		return "", false, err
	}
	if !launched {
		fmt.Fprintf(cmd.OutOrStdout(), "File: %s\nSet $EDITOR to edit it automatically.\n", realPath)
		return "", false, nil
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", false, err
	}
	if string(data) == body {
		fmt.Fprintln(cmd.OutOrStdout(), "No changes.")
		return "", false, nil
	}
	return string(data), true, nil
}
