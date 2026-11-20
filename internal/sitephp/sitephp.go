// Package sitephp holds how a site's PHP is run that every web server lerd
// runs has to agree on: the request timeout, front-controller routing, and the
// framework's own FastCGI parameters.
package sitephp

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// XdebugDebugRequestTimeout is what a site gets while its PHP version runs
// Xdebug in debug mode: a request stopped at a breakpoint sends nothing, and
// the 60s default would answer 504 over a session that is still alive.
const XdebugDebugRequestTimeout = 3600

// RequestTimeout is the seconds PHP may take to answer: the project's own
// setting, else the global one, else an hour while Xdebug debugs, else 60.
func RequestTimeout(sitePath, phpVersion string) int {
	if sitePath != "" {
		if pc, err := config.LoadProjectConfig(sitePath); err == nil && pc.RequestTimeout > 0 {
			return pc.RequestTimeout
		}
	}
	gc, err := config.LoadGlobal()
	if err != nil {
		return config.DefaultRequestTimeout
	}
	if gc.Nginx.RequestTimeout > 0 {
		return gc.Nginx.RequestTimeout
	}
	if XdebugDebugs(gc.GetXdebugMode(phpVersion)) {
		return XdebugDebugRequestTimeout
	}
	return config.DefaultRequestTimeout
}

// XdebugDebugs reports whether an xdebug.mode value turns on step debugging.
// The value is a comma-separated set, so "develop,debug" debugs as well.
func XdebugDebugs(mode string) bool {
	for _, m := range strings.Split(mode, ",") {
		if strings.TrimSpace(m) == "debug" {
			return true
		}
	}
	return false
}

// FrontController reports whether the site routes all PHP through index.php,
// set by the project's .lerd.yaml or by its framework definition.
func FrontController(site config.Site) bool {
	if pc, err := config.LoadProjectConfig(site.Path); err == nil && pc.FrontController {
		return true
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	return ok && fw.FrontController
}

// Param is one FastCGI parameter, in the order a web server should send it.
type Param struct{ Name, Value string }

var paramName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// valueForbidden are the characters that would let a value break out of an
// nginx directive; a value nginx cannot take is dropped for every server, so
// switching servers never changes what PHP receives.
const valueForbidden = "{};#\n\r\x00"

// FastCGIParams expands the framework's declared parameters, {{root}} and
// {{public}} filled in, sorted by name. Each one dropped is named in warnings.
func FastCGIParams(framework, domain string, params map[string]string, sitePath, publicDir string) (out []Param, warnings []string) {
	docRoot := sitePath
	if publicDir != "" && publicDir != "." {
		docRoot = filepath.Join(sitePath, publicDir)
	}
	for _, name := range slices.Sorted(maps.Keys(params)) {
		value := strings.NewReplacer("{{root}}", sitePath, "{{public}}", docRoot).Replace(params[name])
		switch {
		case !paramName.MatchString(name):
			warnings = append(warnings, fmt.Sprintf("[WARN] dropping %s fastcgi param %q for %s: not a valid name\n", framework, name, domain))
		case strings.Contains(value, "{{"):
			warnings = append(warnings, fmt.Sprintf("[WARN] dropping %s fastcgi param %s for %s: unknown {{placeholder}}\n", framework, name, domain))
		case strings.ContainsAny(value, valueForbidden):
			warnings = append(warnings, fmt.Sprintf("[WARN] dropping %s fastcgi param %s for %s: value would end the directive\n", framework, name, domain))
		// nginx reads $ as a variable even inside quotes: an unknown one fails the
		// config for every site, a known one changes what PHP receives.
		case strings.Contains(value, "$"):
			warnings = append(warnings, fmt.Sprintf("[WARN] dropping %s fastcgi param %s for %s: value contains $, which nginx would read as a variable\n", framework, name, domain))
		default:
			out = append(out, Param{Name: name, Value: value})
		}
	}
	return out, warnings
}
