package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Jobs a plugin can take over from lerd by declaring itself in charge of them
// under provides:. Lerd then hands the job to the plugin instead of doing it.
const (
	// JobWorktreeDatabase creates and drops a worktree's isolated database.
	JobWorktreeDatabase = "worktree_database"
)

var jobs = map[string]bool{JobWorktreeDatabase: true}

// SurfaceProvide is the surface a plugin's executable runs under when lerd
// hands it a job.
const SurfaceProvide = "provide"

// ProviderOf returns the installed plugin in charge of job, or nil when lerd
// does the job itself.
func ProviderOf(job string) *Plugin {
	loaded, _ := Load()
	for _, p := range loaded {
		if _, ok := p.Provides[job]; ok {
			return p
		}
	}
	return nil
}

// Provide runs the plugin's executable for job as `<run> <action>`, with the
// request as JSON on stdin, and returns what it printed. A failure carries
// what the plugin wrote to stderr, since that is the only reason anyone gets.
func (p *Plugin) Provide(job, action string, request map[string]string) ([]byte, error) {
	run, ok := p.Provides[job]
	if !ok {
		return nil, fmt.Errorf("plugin %s is not in charge of %s", p.Name, job)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cmd, err := p.Command(run, SurfaceProvide, []string{action}, Env(request))
	if err != nil {
		return nil, err
	}
	cmd.Dir = p.Dir
	cmd.Stdin = bytes.NewReader(body)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = RunWithin(cmd, provideLimit)
	if err != nil {
		Failed(Failure{Plugin: p.Name, Surface: SurfaceProvide, What: job + " " + action, Error: strings.TrimSpace(stderr.String() + "\n" + err.Error())})
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("plugin %s %s %s: %s", p.Name, job, action, msg)
		}
		return nil, fmt.Errorf("plugin %s %s %s: %w", p.Name, job, action, err)
	}
	return out.Bytes(), nil
}

// checkJobs refuses a plugin taking a job another installed plugin is already
// in charge of; each would assume the other is not there.
func checkJobs(dir, name string) error {
	p, err := ReadPlugin(dir)
	if err != nil {
		return err
	}
	for job := range p.Provides {
		if other := ProviderOf(job); other != nil && other.Name != name {
			return fmt.Errorf("plugin %s wants to be in charge of %s, which plugin %s already is", name, job, other.Name)
		}
	}
	return nil
}
