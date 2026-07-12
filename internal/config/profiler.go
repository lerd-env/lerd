package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrGenerateProfilerKey returns the SPX HTTP key, generating and
// persisting a fresh random one on first use. The key gates the SPX profiler
// UI and per-request profiling. Lerd injects it into the FPM HTTP_COOKIE via
// nginx so it never has to live in a browser cookie (which would be a blocked
// third-party cookie inside the dashboard iframe).
func LoadOrGenerateProfilerKey() (string, error) {
	path := SpxKeyFile()
	if b, err := os.ReadFile(path); err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating profiler key: %w", err)
	}
	key := hex.EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	guardRealWrite(path)
	if err := os.WriteFile(path, []byte(key+"\n"), 0600); err != nil {
		return "", fmt.Errorf("writing profiler key: %w", err)
	}
	return key, nil
}
