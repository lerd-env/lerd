// Package hostshell runs a command line on the host the way a user would type
// it: through sh on Linux and macOS, and through PowerShell on Windows, which
// has no sh. Site commands, doctor fixes and worktree migrations are strings
// written for a shell, so every place that runs one on the host goes through
// here. Commands run inside containers keep using sh, which they always have.
package hostshell
