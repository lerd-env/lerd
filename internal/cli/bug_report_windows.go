package cli

import "io"

// writeHostDetails adds nothing on Windows beyond the OS/arch line.
func writeHostDetails(io.Writer) {}
