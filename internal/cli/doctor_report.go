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
// order. Callers filter further (heavy fixes always re-confirm).
func (r *DoctorReport) AutoFixes() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Fix != nil && f.Fix.Tier == FixAuto {
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
