package config

import (
	"fmt"
	"regexp"
	"strings"
)

// A worker's tune_command is the same command as its plain one with the values
// a project may want to change written as {placeholders}. Everything here reads
// that template: which knobs a worker offers, what the definition already runs
// for each, and the command a set of values renders to. The store stays the
// single source of truth, so a framework that declares a new placeholder gets
// the flag, the dashboard field and the .lerd.yaml key for free.

// WorkerTuneFlag is one value a worker's tune_command declares.
type WorkerTuneFlag struct {
	Name    string
	Default string
}

var tunePlaceholderRe = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9_-]*)\}`)

// WorkerTuneFlags reads the values a worker declares through its tune_command
// placeholders, in the order they appear. Each default is recovered by matching
// the template against the plain command, so --queue offers whatever the
// definition already runs without the value being repeated anywhere. A
// placeholder the plain command has no counterpart for gets no default and has
// to be passed.
func WorkerTuneFlags(w FrameworkWorker) []WorkerTuneFlag {
	if w.TuneCommand == "" {
		return nil
	}
	var names, segments []string
	rest := w.TuneCommand
	for {
		loc := tunePlaceholderRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			segments = append(segments, rest)
			break
		}
		segments = append(segments, rest[:loc[0]])
		names = append(names, rest[loc[2]:loc[3]])
		rest = rest[loc[1]:]
	}

	flags := make([]WorkerTuneFlag, 0, len(names))
	pos := 0
	for i, name := range names {
		var def string
		def, pos = tuneDefault(w.Command, segments[i], segments[i+1], pos)
		flags = append(flags, WorkerTuneFlag{Name: name, Default: def})
	}
	return flags
}

// tuneDefault recovers one placeholder's value from the plain command by
// locating the literal text the template puts around it. Returns "" and -1 when
// that text isn't there, which ends the scan: without a fixed point the later
// placeholders can't be located either.
func tuneDefault(command, before, after string, from int) (string, int) {
	if from < 0 || from > len(command) {
		return "", -1
	}
	idx := strings.Index(command[from:], before)
	if idx < 0 {
		return "", -1
	}
	start := from + idx + len(before)
	if after == "" {
		return command[start:], len(command)
	}
	end := strings.Index(command[start:], after)
	if end < 0 {
		// The template says more than the plain command does (CodeIgniter takes
		// the queue positionally and never spells its -tries= default), so the
		// rest of the command is this placeholder's value and the ones after it
		// have no default.
		return command[start:], len(command)
	}
	return command[start : start+end], start + end
}

// TuneValues returns the values a project actually runs a worker with: the
// defaults its tune_command declares, with whatever the project persisted for it
// layered on top.
func TuneValues(dir, name string, w FrameworkWorker) map[string]string {
	flags := WorkerTuneFlags(w)
	if len(flags) == 0 {
		return nil
	}
	values := make(map[string]string, len(flags))
	for _, f := range flags {
		if f.Default != "" {
			values[f.Name] = f.Default
		}
	}
	for k, v := range ProjectWorkerOptions(dir, name) {
		if v != "" {
			values[k] = v
		}
	}
	return values
}

// ExpandTunePlaceholders fills a worker's {placeholder} tokens in s. A token
// with no value is left as it is, so a caller never silently routes somewhere
// the worker is not.
func ExpandTunePlaceholders(s string, values map[string]string) string {
	if s == "" || len(values) == 0 {
		return s
	}
	return tunePlaceholderRe.ReplaceAllStringFunc(s, func(token string) string {
		if v := values[strings.Trim(token, "{}")]; v != "" {
			return v
		}
		return token
	})
}

// RenderTuneCommand substitutes values into the worker's tune_command. With
// nothing overridden the plain command is returned verbatim, so a start with no
// options runs exactly what the definition declares.
func RenderTuneCommand(w FrameworkWorker, values map[string]string) (string, error) {
	if w.TuneCommand == "" || len(values) == 0 {
		return w.Command, nil
	}
	command := w.TuneCommand
	for _, f := range WorkerTuneFlags(w) {
		value := values[f.Name]
		if value == "" {
			value = f.Default
		}
		if value == "" {
			return "", fmt.Errorf("%s must be given: the framework definition declares no default for {%s}", f.Name, f.Name)
		}
		// The value is interpolated into the command the worker's unit runs, so
		// whitespace or a newline could add an argument or a systemd directive.
		if strings.ContainsAny(value, " \t\r\n") {
			return "", fmt.Errorf("invalid %s value: must not contain whitespace", f.Name)
		}
		command = strings.ReplaceAll(command, "{"+f.Name+"}", value)
	}
	return command, nil
}

// WorkerTuneOption is one tunable value paired with what the project committed
// for it, which is what a UI needs to render the knob.
type WorkerTuneOption struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default string `json:"default"`
}

// WorkerTuneOptions returns every value the worker offers for a site: the
// placeholder, the default the definition runs, and the value the project
// persisted in .lerd.yaml. Nil for a worker that declares no tune_command.
func WorkerTuneOptions(sitePath, workerName string, w FrameworkWorker) []WorkerTuneOption {
	flags := WorkerTuneFlags(w)
	if len(flags) == 0 {
		return nil
	}
	values := ProjectWorkerOptions(sitePath, workerName)
	opts := make([]WorkerTuneOption, 0, len(flags))
	for _, f := range flags {
		opts = append(opts, WorkerTuneOption{Name: f.Name, Value: values[f.Name], Default: f.Default})
	}
	return opts
}
