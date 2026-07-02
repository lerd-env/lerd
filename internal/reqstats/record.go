// Package reqstats turns the nginx access feed into a per-site view of request
// timing: the typical response time and the routes that run well above their own
// baseline. It consumes the same syslog datagrams that drive idle-suspend, but a
// widened log format, and keeps a small rolling in-memory window per route. It is
// framework-agnostic: nothing here knows a framework name, only universal nginx
// timing signals.
package reqstats

import (
	"strconv"
	"strings"
)

// AccessRecord is one parsed nginx access datagram. The feed emits a
// pipe-delimited message "$host|$status|$request_time|$request_method|$request_uri"
// as the final whitespace token, so it survives syslog framing the same way the
// idle host-only format does.
type AccessRecord struct {
	Host        string
	Status      int
	RequestTime float64 // seconds, as nginx $request_time
	Method      string
	URI         string // raw $request_uri, query string included
}

// SecondsToMillis returns the request time in milliseconds.
func (r AccessRecord) SecondsToMillis() float64 { return r.RequestTime * 1000 }

// fieldCount is the number of pipe-delimited fields in a timing message. URI is
// last so a literal pipe inside it (rare, usually %7C-encoded) stays intact.
const fieldCount = 5

// ParseAccessRecord extracts a timing record from one access datagram. It takes
// the final whitespace token (skipping syslog framing, as the idle parser does)
// and splits it on '|'. Returns ok=false for the old host-only format, nginx's
// "-" host placeholder, or a line whose status/time don't parse.
func ParseAccessRecord(datagram []byte) (AccessRecord, bool) {
	line := strings.TrimRight(string(datagram), "\n\x00 ")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return AccessRecord{}, false
	}
	parts := strings.SplitN(fields[len(fields)-1], "|", fieldCount)
	if len(parts) != fieldCount {
		return AccessRecord{}, false
	}
	host := parts[0]
	if host == "" || host == "-" {
		return AccessRecord{}, false
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return AccessRecord{}, false
	}
	secs, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return AccessRecord{}, false
	}
	return AccessRecord{
		Host:        host,
		Status:      status,
		RequestTime: secs,
		Method:      parts[3],
		URI:         parts[4],
	}, true
}
