package goserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// lerdParams are the variables lerd's nginx PHP location adds: forwarded host
// and scheme, client, site, hand-over time (debug capture's queue time), and
// the SPX key, only for direct local requests so a share cannot profile.
func lerdParams(r *http.Request, site config.Site) map[string]string {
	host := forwardedHost(r)
	port := r.Header.Get("X-Forwarded-Port")
	if port == "" {
		port = serverPort(r)
	}
	client := remoteIP(r)
	return map[string]string{
		"HTTP_HOST":              host,
		"SERVER_NAME":            host,
		"HTTP_X_FORWARDED_HOST":  host,
		"HTTP_X_FORWARDED_PROTO": forwardedProto(r),
		"HTTP_X_FORWARDED_PORT":  port,
		"HTTP_X_REAL_IP":         client,
		"HTTP_X_FORWARDED_FOR":   client,
		"LERD_NGINX_SENT":        fmt.Sprintf("%.3f", float64(time.Now().UnixMilli())/1000),
		"HTTP_COOKIE":            spxCookie(r),
		"LERD_SITE":              site.Name,
	}
}

func spxCookie(r *http.Request) string {
	key := ""
	if r.Header.Get("X-Forwarded-Host") == "" {
		key, _ = config.LoadOrGenerateProfilerKey()
	}
	cookie := r.Header.Get("Cookie") + "; SPX_KEY=" + key
	if cfg, err := config.LoadGlobal(); err == nil && cfg.IsProfilerEnabled() {
		cookie += "; SPX_ENABLED=1"
	}
	return cookie
}
