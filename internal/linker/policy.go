// Package linker owns the decision half of registering a directory as a lerd
// site. Every caller that links a project — the CLI, the parked-directory
// watcher, the web UI and the MCP server — resolves the same plan here and
// differs only in the capabilities it grants through a Policy.
package linker

import "github.com/geodro/lerd/internal/config"

// Prompter resolves a question a link cannot answer on its own. Callers with a
// terminal supply one; every other caller passes nil, and the link takes the
// conservative branch instead of blocking on input that will never arrive.
type Prompter interface {
	// Confirm asks a yes/no question and reports the answer.
	Confirm(question string, defaultYes bool) bool
	// Choose asks the user to pick one of options and returns its index.
	Choose(title string, options []string) (int, error)
}

// Policy is the capability set a caller grants a link. The zero value is the
// most restrictive one: it registers a site and serves it, and does nothing
// else. Each field widens that.
type Policy struct {
	// Name overrides the site name and primary domain. Empty derives both from
	// the directory name.
	Name string
	// AssumeYes records consent given outside a prompt, such as `lerd link
	// --yes` or the click that started a link from the web UI.
	AssumeYes bool
	// Prompt, when non-nil, may ask the user to resolve a decision. Nil means
	// no question can be asked, whether or not a terminal is attached.
	Prompt Prompter
	// ProjectWrites allows writing into the project directory: the .php-version
	// and .node-version pins, and the .lerd.yaml domain and framework writeback.
	ProjectWrites bool
	// Services allows installing and starting the services the project declares
	// and the ones its framework requires.
	Services bool
	// Certs allows the site to be registered as secured, which issues a
	// certificate for it.
	Certs bool
	// RepoCommands allows running commands the repository authored: a
	// host-proxy dev server and inline service containers. Consent for these
	// still goes through the usual gate; this only says the caller is a context
	// where running them is on the table at all.
	RepoCommands bool
	// ImageBuild allows building or pulling a PHP image the host does not have.
	ImageBuild bool
	// SkipRegistered stops the link when the directory is already a registered
	// site, rather than re-linking it.
	SkipRegistered bool
	// DeferPublish leaves the reloads that publish a link — systemd, the quadlet
	// rewrite, the container hosts file and nginx — to the caller, so a batch of
	// links does that work once at the end instead of once per project.
	DeferPublish bool
}

// CLIPolicy is the policy for a user-invoked `lerd link`: everything is
// permitted, and prompt decides whether questions can be asked.
func CLIPolicy(name string, assumeYes bool, prompt Prompter) Policy {
	return Policy{
		Name:          name,
		AssumeYes:     assumeYes,
		Prompt:        prompt,
		ProjectWrites: true,
		Services:      true,
		Certs:         true,
		RepoCommands:  true,
		ImageBuild:    true,
	}
}

// WatcherPolicy is the policy for `lerd park` and the parked-directory watcher.
// It runs unattended against every subdirectory of a parked tree, so it reads
// the project's committed configuration but never asks a question, never writes
// into the project, and never runs anything the repository authored.
func WatcherPolicy() Policy {
	return Policy{SkipRegistered: true}
}

// Mode is how a linked site is served.
type Mode string

const (
	ModeFPM             Mode = "fpm"
	ModeFrankenPHP      Mode = "frankenphp"
	ModeCustomFPM       Mode = "fpm-custom"
	ModeCustomContainer Mode = "container"
	ModeHostProxy       Mode = "host-proxy"
)

// Skip says why a directory will not be registered.
type Skip string

const (
	SkipNone       Skip = ""
	SkipWorktree   Skip = "worktree"
	SkipRegistered Skip = "already-registered"
)

// Plan is everything a link decided before it changed anything. Resolve
// produces it from the directory, the global config and the policy; Apply
// carries it out.
type Plan struct {
	// Dir is the directory being linked, as given.
	Dir string
	// Site is the registration that will be written.
	Site config.Site
	// Mode is how Site will be served.
	Mode Mode
	// Skip is set when the directory will not be registered at all, and
	// SkipDetail explains it in words the caller can print.
	Skip       Skip
	SkipDetail string
	// WorktreeParent and WorktreeBranch are set alongside SkipWorktree: the
	// site that owns this checkout, and the branch it holds.
	WorktreeParent *config.Site
	WorktreeBranch string
	// Project is the parsed .lerd.yaml, or nil when the project has none.
	Project *config.ProjectConfig
	// DroppedDomains lists domains another site already owns, which were
	// filtered out of Site.Domains.
	DroppedDomains []string
	// PHPSuggestion names a PHP version that suits the framework better than
	// the one Site carries. Empty when the chosen version is already the best
	// installed one, or when the policy forbids building images.
	PHPSuggestion string
	// PHPMin and PHPMax are the framework's supported range, for the message
	// that accompanies a clamped version. Both empty when unconstrained.
	PHPMin, PHPMax string
	// FrameworkLabel names the framework in user-facing messages, and is
	// "unknown (public: <dir>)" when detection found none.
	FrameworkLabel string
	// FrankenPHPDeclined records that the project asked for FrankenPHP but no
	// image exists for its PHP version, so the site falls back to FPM.
	FrankenPHPDeclined bool
	// ProxyCommand is the host dev-server command awaiting consent, empty when
	// the site runs none.
	ProxyCommand string
}

// Registered reports whether the plan will actually register a site.
func (p *Plan) Registered() bool { return p.Skip == SkipNone }
