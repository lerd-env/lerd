package ui

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// remoteSessionCookie authenticates LAN dashboard requests without a fresh
// HTTP Basic challenge. iOS Safari drops cached Basic credentials between
// refreshes, so relying on the browser to re-send Authorization means the
// native password dialog pops on every page load. Once the user has cleared
// the Basic gate once, we hand them this cookie and accept it in lieu of the
// header on later requests.
const remoteSessionCookie = "lerd_session"

// remoteSessionTTL is how long a session cookie stays valid. A week keeps a
// phone signed in across a normal working stretch while still expiring if a
// device is lost.
const remoteSessionTTL = 7 * 24 * time.Hour

// remoteSessionMAC authenticates the encoded username and expiry with the
// bcrypt password hash as the key. The hash never leaves the server, so a
// client cannot forge a value, and changing or clearing credentials rotates
// the key and invalidates every outstanding cookie.
func remoteSessionMAC(passwordHash, encUser, exp string) string {
	h := hmac.New(sha256.New, []byte(passwordHash))
	h.Write([]byte(encUser + "|" + exp))
	return hex.EncodeToString(h.Sum(nil))
}

// remoteSessionSign returns the cookie value binding username to expiry.
func remoteSessionSign(username, passwordHash string, expiry time.Time) string {
	encUser := base64.RawURLEncoding.EncodeToString([]byte(username))
	exp := strconv.FormatInt(expiry.Unix(), 10)
	return encUser + "." + exp + "." + remoteSessionMAC(passwordHash, encUser, exp)
}

// remoteSessionValid reports whether cookieVal authenticates as the configured
// user. Tampered, expired, or prior-password values all fail.
func remoteSessionValid(cookieVal, username, passwordHash string, now time.Time) bool {
	if passwordHash == "" {
		return false
	}
	parts := strings.Split(cookieVal, ".")
	if len(parts) != 3 {
		return false
	}
	encUser, exp, mac := parts[0], parts[1], parts[2]
	raw, err := base64.RawURLEncoding.DecodeString(encUser)
	if err != nil || string(raw) != username {
		return false
	}
	ts, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || now.After(time.Unix(ts, 0)) {
		return false
	}
	want := remoteSessionMAC(passwordHash, encUser, exp)
	return subtle.ConstantTimeCompare([]byte(mac), []byte(want)) == 1
}

// setRemoteSessionCookie writes a fresh session cookie. HttpOnly and
// SameSite=Lax; not Secure because the LAN dashboard is served over plain
// HTTP at http://<lan-ip>:7073 with no certificate.
func setRemoteSessionCookie(w http.ResponseWriter, username, passwordHash string, now time.Time) {
	exp := now.Add(remoteSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     remoteSessionCookie,
		Value:    remoteSessionSign(username, passwordHash, exp),
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(remoteSessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
