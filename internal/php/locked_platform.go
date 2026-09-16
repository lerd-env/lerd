package php

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// platformCheckFloor pulls the PHP_VERSION_ID floor out of composer's generated
// platform check. The file is machine-written and has exactly one such gate.
var platformCheckFloor = regexp.MustCompile(`PHP_VERSION_ID\s*>=\s*(\d{5,6})`)

// LockedPlatformPHPConstraint returns the PHP floor the project's *resolved*
// dependencies require, as a composer-style constraint, or "" when there is
// none to read.
//
// A project's composer.json says what the app claims to support; the tree
// underneath it can need more. Laravel 13 declares "php": "^8.3" and then
// resolves Symfony 8 components that each require ">=8.4.1", so the app cannot
// boot on 8.3 even though its own manifest allows it. Composer already works
// this out and writes it into vendor/composer/platform_check.php, which is the
// same answer the site would give at the first request:
//
//	Your Composer dependencies require a PHP version ">= 8.4.1".
//
// Reading the lock rather than the manifest is what makes a refusal match what
// the site will actually do. The file is absent when nothing has been installed
// yet, and composer omits the gate when no package constrains PHP, so an empty
// return means "nothing to add", never "unconstrained by accident".
func LockedPlatformPHPConstraint(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "vendor", "composer", "platform_check.php"))
	if err != nil {
		return ""
	}
	m := platformCheckFloor.FindSubmatch(data)
	if m == nil {
		return ""
	}
	id, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return ""
	}
	return ">=" + versionIDToString(id)
}

// versionIDToString turns composer's PHP_VERSION_ID back into a version. The id
// is major*10000 + minor*100 + patch, so the minor and patch components are two
// digits each: 80401 is 8.4.1, not 8.40.1.
func versionIDToString(id int) string {
	major := id / 10000
	minor := (id / 100) % 100
	patch := id % 100
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}
