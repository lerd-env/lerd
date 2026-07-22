package cli

import "encoding/json"

// The doctor produces a structured report alongside its human-readable output
// so `lerd doctor --fix` and the MCP diag tool can act on findings without
// re-parsing text. Each finding may carry a fix classified into one of three
// tiers: Auto is lerd-owned and needs no privilege, Manual needs sudo so lerd
// only prints the command, None is external state lerd will not touch.

// FixTier classifies how a finding can be repaired.
type FixTier int

const (
	// FixNone means the finding has no attached fix (external state or the
	// hint alone tells the user what to do).
	FixNone FixTier = iota
	// FixAuto means lerd can apply the fix itself with no elevated privilege.
	FixAuto
	// FixManual means the fix needs sudo, so lerd shows the command (in the
	// finding's Hint) and never runs it.
	FixManual
)

// MarshalJSON renders the tier as a stable string so machine consumers (the MCP
// diag tool) can branch on "auto"/"manual"/"none" without a magic integer.
func (t FixTier) MarshalJSON() ([]byte, error) {
	switch t {
	case FixAuto:
		return json.Marshal("auto")
	case FixManual:
		return json.Marshal("manual")
	default:
		return json.Marshal("none")
	}
}

// DoctorFix describes how to repair a finding. For FixAuto, Key selects the
// repair the engine runs and Arg carries its target (a path, PHP version, user,
// or TLD); Label is a human description of the action. For FixManual only Tier
// is meaningful and the command lives in the finding's Hint.
type DoctorFix struct {
	Tier  FixTier `json:"tier"`
	Key   string  `json:"key,omitempty"`
	Arg   string  `json:"-"`
	Label string  `json:"label,omitempty"`
}

// Finding is one structured doctor result.
type Finding struct {
	Section string     `json:"section,omitempty"`
	Name    string     `json:"name"`
	Status  string     `json:"status"` // "ok", "fail", "warn", "info"
	Message string     `json:"message,omitempty"`
	Hint    string     `json:"hint,omitempty"`
	Fix     *DoctorFix `json:"fix,omitempty"`
}

// DoctorReport is the structured result of a full doctor run.
type DoctorReport struct {
	Version  string    `json:"version"`
	Findings []Finding `json:"findings"`
	Failures int       `json:"failures"`
	Warnings int       `json:"warnings"`
}

func (r *DoctorReport) add(f Finding) { r.Findings = append(r.Findings, f) }

// fixLast attaches a fix to the most recently added finding, so a check can tag
// itself right after emitting its fail/warn.
func (r *DoctorReport) fixLast(fx *DoctorFix) {
	if n := len(r.Findings); n > 0 {
		r.Findings[n-1].Fix = fx
	}
}

// AutoFixes returns the findings carrying an applicable automatic fix, in report
// order, one per distinct fix. Several failing findings can attach the same
// repair (a broken DNS chain fails at more than one rung), and running that
// repair once per finding would repeat the whole sequence. The identity is the
// key plus its argument: mkdir and php-rebuild reuse one key across findings
// that each target a different directory or PHP version, and those must all
// survive. Callers filter further (heavy fixes always re-confirm).
func (r *DoctorReport) AutoFixes() []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, f := range r.Findings {
		if f.Fix == nil || f.Fix.Tier != FixAuto {
			continue
		}
		id := f.Fix.Key + "\x00" + f.Fix.Arg
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, f)
	}
	return out
}

// RequiredAutoFixes returns the automatic fixes attached to a real problem, and
// OptionalAutoFixes those hanging off an informational finding. An info finding
// is a standing offer, never a repair the user still owes.
func (r *DoctorReport) RequiredAutoFixes() []Finding { return r.autoFixes(false) }

// OptionalAutoFixes returns the automatic fixes attached to info findings.
func (r *DoctorReport) OptionalAutoFixes() []Finding { return r.autoFixes(true) }

func (r *DoctorReport) autoFixes(info bool) []Finding {
	var out []Finding
	for _, f := range r.AutoFixes() {
		if (f.Status == "info") == info {
			out = append(out, f)
		}
	}
	return out
}

// ManualFixes returns the findings whose repair needs sudo, so the CLI can list
// their commands for the user to run.
func (r *DoctorReport) ManualFixes() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Fix != nil && f.Fix.Tier == FixManual {
			out = append(out, f)
		}
	}
	return out
}

func autoFix(key, arg, label string) *DoctorFix {
	return &DoctorFix{Tier: FixAuto, Key: key, Arg: arg, Label: label}
}

var manualFix = &DoctorFix{Tier: FixManual}

// manualFixWith is manualFix carrying the command to run, for repairs lerd
// knows the fix for but will not run itself because it needs sudo.
func manualFixWith(label string) *DoctorFix {
	return &DoctorFix{Tier: FixManual, Label: label}
}
