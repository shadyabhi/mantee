package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

// UIState represents the current state of the UI
type UIState int

const (
	StateInput    UIState = iota // Text input for search term
	StateSelect                  // Selection list for results
)

// Model represents the Bubble Tea model
type Model struct {
	state        UIState
	input        string
	pages        []ManPage
	cursor       int
	scrollOffset int // Scroll offset for viewport
	selected     *ManPage
	quitting     bool
	keyword      string
	err          string
	width        int
	height       int
}

// NewInputModel creates a new Model starting with text input
func NewInputModel() Model {
	return Model{
		state: StateInput,
	}
}

// NewSelectModel creates a new Model starting with selection (when keyword provided via CLI)
func NewSelectModel(keyword string, pages []ManPage) Model {
	return Model{
		state:   StateSelect,
		pages:   pages,
		cursor:  0,
		keyword: keyword,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.state {
		case StateInput:
			return m.updateInput(msg)
		case StateSelect:
			return m.updateSelect(msg)
		}
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit

	case "enter":
		if m.input == "" {
			return m, nil
		}
		// Search for man pages
		pages, err := SearchManPages(m.input)
		if err != nil {
			m.err = fmt.Sprintf("Error searching: %v", err)
			return m, nil
		}
		if len(pages) == 0 {
			m.err = fmt.Sprintf("No man pages found for: %s", m.input)
			return m, nil
		}
		// Transition to selection state
		m.state = StateSelect
		m.keyword = m.input
		m.pages = pages
		m.cursor = 0
		m.scrollOffset = 0 // Reset scroll to top
		m.err = ""
		return m, nil

	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
			m.err = "" // Clear error when typing
		}

	default:
		// Add printable characters to input
		if len(msg.String()) == 1 {
			m.input += msg.String()
			m.err = "" // Clear error when typing
		}
	}
	return m, nil
}

func (m Model) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
		}

	case "down", "j":
		if m.cursor < len(m.pages)-1 {
			m.cursor++
			m.adjustScroll()
		}

	case "enter":
		if len(m.pages) > 0 {
			m.selected = &m.pages[m.cursor]
		}
		return m, tea.Quit

	case "home", "g":
		m.cursor = 0
		m.scrollOffset = 0

	case "end", "G":
		m.cursor = len(m.pages) - 1
		m.adjustScroll()
	}
	return m, nil
}

// viewportHeight returns the number of items that fit in the viewport
func (m Model) viewportHeight() int {
	// Reserve lines for: title (2 lines with spacing), help line (2 lines with spacing)
	reserved := 4
	if m.height <= reserved {
		return 10 // Minimum fallback
	}
	return m.height - reserved
}

// adjustScroll ensures the cursor is visible within the viewport
func (m *Model) adjustScroll() {
	vpHeight := m.viewportHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+vpHeight {
		m.scrollOffset = m.cursor - vpHeight + 1
	}
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting || m.selected != nil {
		return ""
	}

	switch m.state {
	case StateInput:
		return m.viewInput()
	case StateSelect:
		return m.viewSelect()
	}
	return ""
}

func (m Model) viewInput() string {
	s := promptStyle.Render("Search man pages: ") + m.input + "█\n\n"

	if m.err != "" {
		s += errorStyle.Render(m.err) + "\n\n"
	}

	s += helpStyle.Render("enter search • esc quit")
	return s
}

func (m Model) viewSelect() string {
	s := titleStyle.Render(fmt.Sprintf("Search results for: %s", m.keyword)) + "\n\n"

	vpHeight := m.viewportHeight()
	endIdx := m.scrollOffset + vpHeight
	if endIdx > len(m.pages) {
		endIdx = len(m.pages)
	}

	for i := m.scrollOffset; i < endIdx; i++ {
		page := m.pages[i]
		line := page.String()
		if i == m.cursor {
			s += selectedStyle.Render("> "+line) + "\n"
		} else {
			s += normalStyle.Render("  "+line) + "\n"
		}
	}

	s += "\n"
	s += helpStyle.Render(fmt.Sprintf("[%d/%d] ↑/k up • ↓/j down • enter select • q quit", m.cursor+1, len(m.pages)))

	return s
}

// Selected returns the selected man page, or nil if none selected
func (m Model) Selected() *ManPage {
	return m.selected
}
