package config

import "path/filepath"

// installedLerdBinary is lerd.exe in lerd's own bin dir, where install puts it.
func installedLerdBinary() string { return filepath.Join(BinDir(), "lerd.exe") }
