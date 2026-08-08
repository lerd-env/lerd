package envfile

import "fmt"

// Formats lerd can read and write. A definition naming anything else is from a
// newer store than this binary understands.
const (
	FormatDotenv   = "dotenv"
	FormatPhpConst = "php-const"
	FormatPhpArray = "php-array"
	FormatPhpVars  = "php-vars"
)

// KnownFormat reports whether this binary can read and write a format. An empty
// format is dotenv, which is what a definition that names none has always meant.
func KnownFormat(format string) bool {
	switch format {
	case "", FormatDotenv, FormatPhpConst, FormatPhpArray, FormatPhpVars:
		return true
	}
	return false
}

// ApplyUpdatesIn writes updates to an env file in the given format, and is the
// one way anything writes one.
//
// An unknown format is refused rather than written. The store reaches every
// install within a day, whatever binary it runs, so a definition naming a format
// added after a given release will land on machines that cannot honour it. The
// switches this replaces each fell through to the dotenv writer, which appended
// `key=value` lines into whatever file the definition named: a PHP settings file
// so treated stops parsing, and the site is down through no fault of its owner.
// Refusing leaves the project exactly as it was, which is the worst a binary
// too old for its definition should ever do.
func ApplyUpdatesIn(path, format string, updates map[string]string) error {
	switch format {
	case "", FormatDotenv:
		return ApplyUpdates(path, updates)
	case FormatPhpConst:
		return ApplyPhpConstUpdates(path, updates)
	case FormatPhpArray:
		return ApplyPhpArrayUpdates(path, updates)
	case FormatPhpVars:
		return ApplyPhpVarsUpdates(path, updates)
	}
	return fmt.Errorf("env format %q is not one this version of lerd can write; update lerd to use this framework definition", format)
}
