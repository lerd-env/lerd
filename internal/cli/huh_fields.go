package cli

import (
	"slices"

	"charm.land/huh/v2"

	"github.com/geodro/lerd/internal/config"
)

// newMultiSelect builds the wizard's multi-select fields. Pass an empty
// description to leave it off.
func newMultiSelect(title, description string, options []string, value *[]string) *huh.MultiSelect[string] {
	return newMultiSelectOptions(title, description, huh.NewOptions(options...), value)
}

// newMultiSelectOptions is newMultiSelect for options whose label differs from
// their value.
//
// The explicit height is not cosmetic: an unsized huh multi-select sizes its
// viewport to the option count and then subtracts the title and description
// lines, so short lists lose rows and can render nothing at all.
func newMultiSelectOptions(title, description string, options []huh.Option[string], value *[]string) *huh.MultiSelect[string] {
	field := huh.NewMultiSelect[string]().Title(title)
	height := len(options) + 1
	if description != "" {
		field = field.Description(description)
		height++
	}
	return field.Options(options...).Value(value).Height(height)
}

// serviceOptions labels each suggested service with the package behind it, or
// the definition's reason when no package made the suggestion.
func serviceOptions(names []string, suggested []config.ServiceSuggestion) []huh.Option[string] {
	out := make([]huh.Option[string], len(names))
	for i, name := range names {
		label := name
		if j := slices.IndexFunc(suggested, func(sg config.ServiceSuggestion) bool { return sg.Name == name }); j >= 0 {
			why := suggested[j].Package
			if why == "" {
				why = suggested[j].Reason
			}
			if why != "" {
				label += " (" + why + ")"
			}
		}
		out[i] = huh.NewOption(label, name)
	}
	return out
}
