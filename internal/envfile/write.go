package envfile

import "os"

// writeFile writes an env file, restoring write permission for the duration
// when the file has none and putting the mode back exactly as it was.
//
// A framework may harden its own configuration: Drupal's installer leaves
// settings.php read-only, and its directory too, which is correct for a
// deployed site and is not a reason for lerd to refuse to configure a local
// one. Failing with "permission denied" left the user to chmod by hand, work
// out that lerd had already created the databases, and undo the rest.
//
// The mode is restored even when the write fails, so a file lerd could not
// update is left exactly as hardened as it was found.
func writeFile(path string, data []byte, perm os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
		if perm&0o200 == 0 {
			if err := os.Chmod(path, perm|0o200); err != nil {
				return err
			}
			defer func() { _ = os.Chmod(path, perm) }()
		}
	}
	return os.WriteFile(path, data, perm)
}
