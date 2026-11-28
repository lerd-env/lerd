package php

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// LockPHPConstraints lists the distinct PHP requirements of every package in
// the project's composer.lock, dev ones included since composer install takes
// them too. A missing or unreadable lock yields none.
func LockPHPConstraints(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "composer.lock"))
	if err != nil {
		return nil
	}
	type pkg struct {
		Require map[string]string `json:"require"`
	}
	var lock struct {
		Packages    []pkg `json:"packages"`
		PackagesDev []pkg `json:"packages-dev"`
	}
	if json.Unmarshal(data, &lock) != nil {
		return nil
	}
	var out []string
	for _, p := range append(lock.Packages, lock.PackagesDev...) {
		if c := p.Require["php"]; c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
