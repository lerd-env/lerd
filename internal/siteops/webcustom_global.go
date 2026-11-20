package siteops

import (
	"path/filepath"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/webcustom"
)

// globalWebFile is lerd's own web server's settings for every site, applied
// before each site's own file.
func globalWebFile() cfgedit.File {
	return cfgedit.File{
		Path:     webcustom.GlobalPath(),
		BkpDir:   filepath.Join(webcustom.Dir(), "bkp"),
		BkpName:  "global.yaml",
		Template: webGlobalTemplate,
	}
}

const webGlobalTemplate = `# Lerd settings for lerd's own web server, applied to every site.
#
# A site's own settings are applied after these and win on the same key.
# Read on every request, so a save takes effect at once.
#
# headers:
#   X-Content-Type-Options: nosniff
# max_body_size: 100m
`

// ReadGlobalWeb returns the global settings, or the seeded template.
func ReadGlobalWeb() (cfgedit.Content, error) { return globalWebFile().Read() }

// SaveGlobalWeb writes the global settings, refusing what the server cannot
// serve. Nothing reloads: the server rereads the file per request.
func SaveGlobalWeb(content string, backup bool) (cfgedit.SaveResult, error) {
	return globalWebFile().Save(content, cfgedit.SaveOpts{Backup: backup, Validate: webValidate})
}

func ResetGlobalWeb() error { return globalWebFile().Reset(NginxWebScope.apply()) }

func ListGlobalWebBackups() ([]cfgedit.Backup, error) { return globalWebFile().ListBackups() }

func ReadGlobalWebBackup(name string) ([]byte, error) { return globalWebFile().ReadBackup(name) }

func RestoreGlobalWeb(name string) (cfgedit.RestoreResult, error) {
	return globalWebFile().Restore(name, NginxWebScope.apply())
}

// ValidGlobalWebBackupName reports whether name is one of the global file's
// backups, so a restore can never reach outside its backup directory.
func ValidGlobalWebBackupName(name string) bool { return globalWebFile().ValidBackupName(name) }
