package man

import (
	"strings"

	"charm.land/glamour/v2"
)

// RenderMarkdown renders markdown content to ANSI-formatted terminal output.
// style should be a glamour standard style name ("dark", "light", "dracula", etc.).
func RenderMarkdown(content string, width int, style string) ([]string, error) {
	if width <= 0 {
		width = 80
	}
	if style == "" {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.Split(content, "\n"), err
	}
	out, err := r.Render(content)
	if err != nil {
		return strings.Split(content, "\n"), err
	}
	return strings.Split(out, "\n"), nil
}
