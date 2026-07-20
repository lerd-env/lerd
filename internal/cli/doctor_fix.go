package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/feedback"
)

// Automatic fix keys. Each is attached to a finding by the doctor and dispatched
// here. Trivial repairs run in-process; the orchestration-heavy ones re-enter
// the same lerd subcommand the user would run, so there is one code path.
const (
	fixMkdir        = "mkdir"
	fixEnableLinger = "enable-linger"
	fixPhpRebuild   = "php-rebuild"
	fixStart        = "start"
	fixInstall      = "install"
	fixCleanup      = "cleanup"
)

// Repairs that need elevated privilege. They are named so the auto tier can be
// tested against them, but they are deliberately absent from ApplyDoctorFix:
// both shell into subcommands that run sudo, which the auto tier promises not
// to do, so the doctor reports them as manual for the user to run.
const (
	fixWSLSetup  = "wsl-setup"
	fixDNSRepair = "dns-repair"
)

// heavyFixKeys are auto fixes that rebuild images, reinstall units, or delete
// data, so `lerd doctor --fix` re-confirms them even under --yes.
var heavyFixKeys = map[string]bool{
	fixInstall: true,
	fixCleanup: true,
}

// reCheckReport re-runs the diagnosis after fixes are applied; a package var so
// tests can stub it instead of probing the live system.
var reCheckReport = RunDoctorReport

// IsHeavyFix reports whether a fix always warrants an extra confirmation.
func IsHeavyFix(fix *DoctorFix) bool {
	return fix != nil && heavyFixKeys[fix.Key]
}

// runDoctorFix drives the interactive repair flow after a diagnosis: it offers
// each automatic fix (confirming per fix, heavy ones even under yes), lists the
// sudo commands the user must run themselves, then re-checks.
func runDoctorFix(w io.Writer, rep DoctorReport, yes, dryRun bool) error {
	required := rep.RequiredAutoFixes()
	optional := rep.OptionalAutoFixes()
	autos := append(append([]Finding{}, required...), optional...)
	manuals := rep.ManualFixes()

	if len(autos) == 0 && len(manuals) == 0 {
		fmt.Fprintln(w, "\nNothing to fix automatically, remaining findings are external state.")
		return nil
	}

	listFixes(w, "Automatic fixes available:", required)
	listFixes(w, "Optional, nothing is wrong with these:", optional)

	if dryRun {
		printManualFixes(w, manuals)
		fmt.Fprintln(w, "\nDry run, nothing was changed.")
		return nil
	}

	applied, failed, skipped := 0, 0, 0
	for _, f := range autos {
		heavy := IsHeavyFix(f.Fix)
		if heavy || !yes {
			q := fmt.Sprintf("Apply fix for %q (%s)?", f.Name, f.Fix.Label)
			if heavy {
				q = fmt.Sprintf("Apply fix for %q (%s)? This can rebuild images, reinstall units, or delete data.", f.Name, f.Fix.Label)
			}
			if !feedback.Confirm(q, !heavy) {
				skipped++
				fmt.Fprintf(w, "  skipped %s\n", f.Name)
				continue
			}
		}
		fmt.Fprintf(w, "\n→ %s\n", f.Fix.Label)
		if err := ApplyDoctorFix(f.Fix, w); err != nil {
			failed++
			fmt.Fprintf(w, "  fix failed: %v\n", err)
			continue
		}
		applied++
	}

	printManualFixes(w, manuals)

	fmt.Fprintf(w, "\nApplied %d fix(es)", applied)
	if skipped > 0 {
		fmt.Fprintf(w, ", skipped %d", skipped)
	}
	if failed > 0 {
		fmt.Fprintf(w, ", %d failed", failed)
	}
	fmt.Fprintln(w, ".")

	if applied > 0 {
		if after, err := reCheckReport(); err == nil {
			if remaining := len(after.RequiredAutoFixes()); remaining == 0 {
				fmt.Fprintln(w, "Re-checked: nothing left to repair.")
			} else {
				fmt.Fprintf(w, "Re-checked: %d automatic fix(es) still needed, run `lerd doctor --fix` again.\n", remaining)
			}
		}
	}
	return nil
}

func printManualFixes(w io.Writer, manuals []Finding) {
	if len(manuals) == 0 {
		return
	}
	fmt.Fprintln(w, "\nThese need elevated privileges, run them yourself:")
	for _, f := range manuals {
		// A fix label names the exact command; otherwise warn-level findings
		// carry their guidance in Message and fail-level in Hint.
		guidance := f.Fix.Label
		if guidance == "" {
			guidance = f.Hint
		}
		if guidance == "" {
			guidance = f.Message
		}
		fmt.Fprintf(w, "  • %s: %s\n", f.Name, guidance)
	}
}

// ApplyDoctorFix runs one automatic fix, streaming its output to out. It never
// runs manual (sudo) fixes; callers show those commands instead.
func ApplyDoctorFix(fix *DoctorFix, out io.Writer) error {
	if fix == nil || fix.Tier != FixAuto {
		return fmt.Errorf("no automatic fix to apply")
	}
	switch fix.Key {
	case fixMkdir:
		if fix.Arg == "" {
			return fmt.Errorf("mkdir fix: no target directory")
		}
		fmt.Fprintf(out, "creating %s\n", fix.Arg)
		return os.MkdirAll(fix.Arg, 0o755)
	case fixEnableLinger:
		if fix.Arg == "" {
			return fmt.Errorf("enable-linger fix: no user")
		}
		return runFixCommand(out, "loginctl", "enable-linger", fix.Arg)
	case fixPhpRebuild:
		if fix.Arg == "" {
			return fmt.Errorf("php-rebuild fix: no version")
		}
		return runSelf(out, "php:rebuild", fix.Arg)
	case fixStart:
		return runSelf(out, "start")
	case fixInstall:
		return runSelf(out, "install")
	case fixCleanup:
		return runSelf(out, "cleanup", "--yes")
	default:
		return fmt.Errorf("unknown fix %q", fix.Key)
	}
}

func listFixes(w io.Writer, heading string, fs []Finding) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", heading)
	for _, f := range fs {
		fmt.Fprintf(w, "  • %s: %s\n", f.Name, f.Fix.Label)
	}
}

func runFixCommand(out io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// runSelf re-enters the running lerd binary so a fix reuses the exact code path
// of the subcommand it stands for.
func runSelf(out io.Writer, args ...string) error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	return runFixCommand(out, self, args...)
}
