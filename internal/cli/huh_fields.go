package cli

import "charm.land/huh/v2"

// newMultiSelect builds the wizard's multi-select fields. Pass an empty
// description to leave it off.
//
// The explicit height is not cosmetic: an unsized huh multi-select sizes its
// viewport to the option count and then subtracts the title and description
// lines, so short lists lose rows and can render nothing at all.
func newMultiSelect(title, description string, options []string, value *[]string) *huh.MultiSelect[string] {
	field := huh.NewMultiSelect[string]().Title(title)
	height := len(options) + 1
	if description != "" {
		field = field.Description(description)
		height++
	}
	return field.Options(huh.NewOptions(options...)...).Value(value).Height(height)
}
