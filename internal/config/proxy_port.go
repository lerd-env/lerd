package config

import (
	"regexp"
	"strconv"
)

// CanMovePort reports whether the dev server may start on another port: its
// framework says it binds the injected one, and the command names no port.
func (p *ProxyConfig) CanMovePort(fw *Framework) bool {
	if fw == nil || fw.DevServer == nil || !fw.DevServer.MovePort || p.Command == "" {
		return false
	}
	named := regexp.MustCompile(`(^|\D)` + strconv.Itoa(p.Port) + `(\D|$)`)
	return !named.MatchString(p.Command)
}
