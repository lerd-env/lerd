package plugins

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// TokenHeader carries a plugin's token on its calls to lerd's API.
const TokenHeader = "X-Lerd-Plugin-Token"

// APIURL is where a plugin reaches lerd's API, lerd-ui's loopback listener
// (internal/ui listenAddr).
const APIURL = "http://127.0.0.1:7073"

func secretPath() string { return filepath.Join(config.DataDir(), "plugin-secret") }

// Token is the plugin's API token: an HMAC of its name under a secret only the
// user can read, so lerd can tell which plugin made a call without storing a
// token per plugin. The secret is created on first use.
func Token(name string) (string, error) {
	secret, err := os.ReadFile(secretPath())
	if errors.Is(err, fs.ErrNotExist) {
		secret, err = createSecret()
	}
	if err != nil {
		return "", err
	}
	return sign(secret, name), nil
}

// PluginForToken returns the installed plugin the token belongs to, or "".
func PluginForToken(token string) string {
	if token == "" {
		return ""
	}
	secret, err := os.ReadFile(secretPath())
	if err != nil {
		return ""
	}
	list, _ := Load()
	for _, p := range list {
		if hmac.Equal([]byte(token), []byte(sign(secret, p.Name))) {
			return p.Name
		}
	}
	return ""
}

func sign(secret []byte, name string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(name))
	return hex.EncodeToString(mac.Sum(nil))
}

func createSecret() ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	path := secretPath()
	config.GuardRealWrite(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// O_EXCL so two processes racing to create it agree on one secret.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(secret); err != nil {
		return nil, err
	}
	return secret, nil
}
