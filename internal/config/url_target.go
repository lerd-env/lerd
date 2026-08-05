package config

import (
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// URLTargetFor resolves where a project's base URL lives: the env file and the
// key inside it, both declared by the framework definition. Laravel's APP_URL in
// .env is only the default; Symfony keeps DEFAULT_URI in .env.local, and a
// definition can opt out entirely with url_key: none (Magento stores its base
// URL in the database). Callers that write the URL must go through this so the
// behaviour stays in the store rather than in Go.
//
// A returned key of "" means the framework has no env-held URL to write.
func URLTargetFor(path string) (envFile, urlKey string) {
	envFile, urlKey = ".env", "APP_URL"
	name, ok := DetectFrameworkForDir(path)
	if !ok {
		return envFile, urlKey
	}
	fw, ok := GetFrameworkForDir(name, path)
	if !ok || fw == nil {
		return envFile, urlKey
	}
	if fw.Env.File != "" {
		envFile = fw.Env.File
	}
	if fw.Env.URLKey != "" {
		urlKey = fw.Env.URLKey
	}
	if strings.EqualFold(urlKey, "none") {
		return envFile, ""
	}
	return envFile, urlKey
}

// SyncSiteURL rewrites the project's base URL and its Vite/Reverb keys for the
// current domain and TLS state, resolving the key and the file from the
// framework definition. Callers securing or renaming a site go through this
// rather than envfile directly, so no path hardcodes Laravel's APP_URL.
func SyncSiteURL(projectPath, domain string, secured bool) error {
	file, key := URLTargetFor(projectPath)
	return envfile.SyncPrimaryDomain(projectPath, file, key, domain, secured)
}

// SetSiteURL writes scheme://domain to the project's declared URL key.
func SetSiteURL(projectPath, scheme, domain string) error {
	file, key := URLTargetFor(projectPath)
	return envfile.UpdateAppURL(projectPath, file, key, scheme, domain)
}
