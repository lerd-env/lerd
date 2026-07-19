package man

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	sectionStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("243"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	divColor      = lipgloss.Color("237")
)

type viewState int

const (
	stateList viewState = iota
	stateDetail
)

// renderDoneMsg carries the result of an async glamour render.
type renderDoneMsg struct {
	path  string
	lines []string
}

// renderCmd runs glamour rendering off the main loop and returns a renderDoneMsg.
func renderCmd(path, content string, width int, style string) tea.Cmd {
	return func() tea.Msg {
		lines, _ := RenderMarkdown(content, width, style)
		return renderDoneMsg{path: path, lines: lines}
	}
}

// Model is the bubbletea model for the documentation browser.
type Model struct {
	allPages  []Page
	filtered  []Page
	cursor    int
	filter    string
	state     viewState
	detailIdx int
	rendered  []string
	loading   bool
	scroll    int
	termW     int
	termH     int
	glamStyle string
	cache     map[string][]string // path → rendered lines
}

// NewModel creates a new Model. If args contains a page slug, that page is opened directly.
// glamStyle should be determined before the TUI starts (e.g. "dark" or "light").
func NewModel(pages []Page, args []string, glamStyle string) Model {
	m := Model{
		allPages:  pages,
		filtered:  pages,
		termW:     80,
		termH:     24,
		glamStyle: glamStyle,
		cache:     make(map[string][]string),
	}

	if len(args) > 0 {
		query := args[0]
		for i, p := range pages {
			if p.Slug == query || p.Section+"/"+p.Slug == query {
				m.detailIdx = i
				m.state = stateDetail
				m.loading = true
				return m
			}
		}
		m.filter = query
		m.filtered = FilterPages(pages, query)
	}
	return m
}

// Init implements tea.Model. Pre-renders all pages concurrently so the cache
// is warm by the time the user opens any page.
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.allPages))
	for i, p := range m.allPages {
		cmds[i] = renderCmd(p.Path, p.Content(), m.termW, m.glamStyle)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case renderDoneMsg:
		m.cache[msg.path] = msg.lines
		// If this is the page currently being displayed, apply it
		if m.loading && m.state == stateDetail && m.allPages[m.detailIdx].Path == msg.path {
			m.rendered = msg.lines
			m.loading = false
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		if m.state == stateDetail && !m.loading {
			m.loading = true
			return m, renderCmd(
				m.allPages[m.detailIdx].Path,
				m.allPages[m.detailIdx].Content(),
				m.termW, m.glamStyle,
			)
		}
		return m, nil

	case tea.KeyPressMsg:
		switch m.state {
		case stateList:
			return m.updateList(msg)
		case stateDetail:
			if m.loading {
				if msg.String() == "ctrl+c" {
					return m, tea.Quit
				}
				if msg.String() == "q" || msg.String() == "esc" {
					m.state = stateList
					m.loading = false
					m.rendered = nil
					return m, nil
				}
				return m, nil
			}
			return m.updateDetail(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}

	case "enter":
		if len(m.filtered) > 0 {
			selectedPath := m.filtered[m.cursor].Path
			for i, p := range m.allPages {
				if p.Path == selectedPath {
					m.detailIdx = i
					break
				}
			}
			m.state = stateDetail
			m.scroll = 0
			if lines, ok := m.cache[m.allPages[m.detailIdx].Path]; ok {
				m.rendered = lines
				m.loading = false
			} else {
				m.loading = true
				m.rendered = nil
				return m, renderCmd(
					m.allPages[m.detailIdx].Path,
					m.allPages[m.detailIdx].Content(),
					m.termW, m.glamStyle,
				)
			}
		}

	case "backspace":
		if len(m.filter) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.filtered = FilterPages(m.allPages, m.filter)
			if m.cursor >= len(m.filtered) {
				m.cursor = max(0, len(m.filtered)-1)
			}
		}

	default:
		if msg.Text != "" {
			m.filter += msg.Text
			m.filtered = FilterPages(m.allPages, m.filter)
			m.cursor = 0
		}
	}
	return m, nil
}

func (m Model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	vh := m.visibleHeight()
	maxScroll := max(0, len(m.rendered)-vh)

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		m.state = stateList
		m.scroll = 0
		m.rendered = nil
	case "down", "j":
		if m.scroll < maxScroll {
			m.scroll++
		}
	case "up", "k":
		if m.scroll > 0 {
			m.scroll--
		}
	case "pgdown", " ":
		m.scroll = min(m.scroll+vh/2, maxScroll)
	case "pgup":
		m.scroll = max(0, m.scroll-vh/2)
	case "home", "g":
		m.scroll = 0
	case "end", "G":
		m.scroll = maxScroll
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var s string
	switch m.state {
	case stateList:
		s = m.viewList()
	case stateDetail:
		s = m.viewDetail()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m Model) visibleHeight() int {
	h := m.termH - 4
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) divider() string {
	w := m.termW
	if w <= 0 {
		w = 80
	}
	return lipgloss.NewStyle().Foreground(divColor).Render(strings.Repeat("─", w))
}

func (m Model) viewList() string {
	var b strings.Builder

	// Header (3 lines)
	b.WriteString("\n")
	b.WriteString("  " + titleStyle.Render("Lerd Documentation") + "\n")
	b.WriteString(m.divider() + "\n")

	// Filter line (1 line)
	b.WriteString("  Filter: " + m.filter + "▌\n")

	// Available lines for list content
	availH := m.termH - 7
	if availH < 3 {
		availH = 3
	}

	if len(m.filtered) == 0 {
		b.WriteString("\n  " + dimStyle.Render("No pages match your filter.") + "\n")
		for i := 2; i < availH; i++ {
			b.WriteString("\n")
		}
	} else {
		type lineInfo struct {
			text    string
			pageIdx int // -1 for non-page lines
		}
		var lines []lineInfo
		lastSection := "SENTINEL"
		for i, page := range m.filtered {
			if page.Section != lastSection {
				if lastSection != "SENTINEL" {
					lines = append(lines, lineInfo{text: "", pageIdx: -1})
				}
				lines = append(lines, lineInfo{
					text:    "  " + sectionStyle.Render(SectionLabel(page.Section)),
					pageIdx: -1,
				})
				lastSection = page.Section
			}
			text := "    " + m.filtered[i].Title
			if i == m.cursor {
				text = "  " + selectedStyle.Render("> "+m.filtered[i].Title)
			}
			lines = append(lines, lineInfo{text: text, pageIdx: i})
		}

		cursorLine := 0
		for i, l := range lines {
			if l.pageIdx == m.cursor {
				cursorLine = i
				break
			}
		}

		start := cursorLine - availH/2
		if start < 0 {
			start = 0
		}
		if start+availH > len(lines) {
			start = max(0, len(lines)-availH)
		}
		end := min(start+availH, len(lines))

		for i := start; i < end; i++ {
			b.WriteString(lines[i].text + "\n")
		}
		for i := end - start; i < availH; i++ {
			b.WriteString("\n")
		}
	}

	// Footer (2 lines)
	b.WriteString(m.divider() + "\n")
	b.WriteString("  " + dimStyle.Render("↑↓/jk navigate   Enter open   Backspace delete filter   q quit") + "\n")

	return b.String()
}

func (m Model) viewDetail() string {
	if m.detailIdx >= len(m.allPages) {
		return ""
	}
	page := m.allPages[m.detailIdx]

	var b strings.Builder

	// Header
	b.WriteString("\n")
	header := fmt.Sprintf("  %s", page.Title)
	if page.Section != "" {
		header += "  " + dimStyle.Render("["+SectionLabel(page.Section)+"]")
	}
	b.WriteString(titleStyle.Render(header) + "\n")
	b.WriteString(m.divider() + "\n")

	if m.loading {
		b.WriteString("\n  " + dimStyle.Render("Rendering…") + "\n")
		return b.String()
	}

	// Content
	vh := m.visibleHeight()
	end := min(m.scroll+vh, len(m.rendered))
	lines := m.rendered[m.scroll:end]
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	for i := len(lines); i < vh; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(m.divider() + "\n")
	scrollPct := 0
	if len(m.rendered) > 0 {
		scrollPct = min((m.scroll+vh)*100/len(m.rendered), 100)
	}
	b.WriteString("  " + dimStyle.Render(fmt.Sprintf(
		"↑↓/jk scroll   PgUp/PgDn page   g/G top/bottom   Esc/q back   %d%%",
		scrollPct,
	)) + "\n")

	return b.String()
}
