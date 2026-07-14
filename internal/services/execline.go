package services

import "strings"

// SplitExecStart splits a systemd ExecStart= line into argv the way systemd
// does, honouring single and double quotes so an argument may contain spaces.
// The launchd backend translates the same unit files systemd reads on Linux, so
// it has to agree on the quoting: a site under a path with a space passes its
// working directory as `-w '/Users/me/My Projects/shop'`, and splitting that on
// whitespace alone would hand podman `Projects/shop'` as the container name.
func SplitExecStart(line string) []string {
	var (
		args    []string
		cur     strings.Builder
		inArg   bool
		quote   rune // 0 when unquoted, else the open quote
		escaped bool
	)
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			// Escapes work outside quotes and inside double quotes, as they do
			// in systemd and the shell. ShellQuote writes an apostrophe in a
			// path as '\'', which only reassembles if this is honoured.
			escaped = true
			inArg = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args
}
