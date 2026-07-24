package cli

import (
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/linker"

	"charm.land/huh/v2"
)

// cliPrompter answers a link's questions at the terminal.
type cliPrompter struct{}

func (cliPrompter) Confirm(question string, defaultYes bool) bool {
	return feedback.Confirm(question, defaultYes)
}

func (cliPrompter) Choose(title string, options []string) (int, error) {
	choice := 0
	opts := make([]huh.Option[int], 0, len(options))
	for i, o := range options {
		opts = append(opts, huh.NewOption(o, i))
	}
	err := huh.NewForm(
		huh.NewGroup(huh.NewSelect[int]().Title(title).Options(opts...).Value(&choice)),
	).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run()
	return choice, err
}

// linkPrompter returns the prompter for the current terminal, or nil when
// there is nobody to ask.
func linkPrompter() linker.Prompter {
	if !isInteractive() {
		return nil
	}
	return cliPrompter{}
}

// cliReporter renders a link's progress through the shared feedback layer.
type cliReporter struct{}

func (cliReporter) Step(label string) linker.Step { return feedbackStep{feedback.Start(label)} }

func (cliReporter) Line(msg string) { feedback.Line(msg) }

func (cliReporter) Warn(format string, a ...any) { feedback.Warn(format, a...) }

func (cliReporter) Val(s string) string { return feedback.Val(s) }

type feedbackStep struct{ s *feedback.Step }

func (f feedbackStep) OK(detail string) { f.s.OK(detail) }
func (f feedbackStep) Fail(err error)   { f.s.Fail(err) }

// linkDeps are the side effects the linker cannot reach from below the cli
// package: image builds, worker supervision, runtime reconciliation and the
// JetBrains data-source sync.
func linkDeps() linker.Deps {
	return linker.Deps{
		EnsureFPMQuadlet:         ensureFPMQuadlet,
		ReconcileRuntimeQuadlets: reconcileStaleRuntimeQuadlets,
		StartHostProxyWorker:     startHostProxyWorker,
		SyncIDEDataSource:        func(root string) bool { return syncIDEDataSource(root).wrote() },
	}
}

// linkPolicy is the capability set a user-invoked link runs under: everything
// is permitted, and the prompter is present only when a terminal is.
func linkPolicy(name string) linker.Policy {
	p := linker.CLIPolicy(name, linkAssumeYes, linkPrompter())
	// The wizard pre-approves the command the user just chose, so the link it
	// triggers treats that as consent already given.
	p.AssumeYes = hostProxyApproved(p.AssumeYes)
	return p
}
