package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewerMode represents the current mode of the viewer
type ViewerMode int

const (
	ModeNormal ViewerMode = iota // Normal viewing mode
	ModeSearch                   // Search/command input mode
)

// SearchType represents what field to search in
type SearchType int

const (
	SearchAll         SearchType = iota // Search both option and description
	SearchOption                        // Search option only (partial match)
	SearchOptionExact                   // Search option only (exact match)
	SearchDescription                   // Search description only
)

// FocusPane represents which pane is currently focused
type FocusPane int

const (
	PaneSidebar FocusPane = iota // Left sidebar with section list
	PaneContent                  // Right content pane
)

// Viewer is the Bubble Tea model for the man page viewer
type Viewer struct {
	content             *ManPageContent
	manPage             ManPage
	mode                ViewerMode
	focusPane           FocusPane  // Which pane is currently focused
	sidebarCursor       int        // Current selection in the sidebar
	sidebarScrollOffset int        // Scroll offset for sidebar
	searchInput         string
	searchQuery         string     // Current active search query
	searchType          SearchType // What to search (all, option, description)
	filteredIndices     []int      // Indices of sections matching the search (for option/desc search)
	matchingLines       []int      // Line numbers matching the search (for full-text search)
	currentMatch        int        // Current match index when navigating
	scrollOffset        int        // Current scroll position
	width               int
	height              int
	quitting            bool
}

// NewViewer creates a new Viewer for the given man page
func NewViewer(page ManPage, content *ManPageContent) Viewer {
	return Viewer{
		content: content,
		manPage: page,
		mode:    ModeNormal,
		width:   80,
		height:  24,
	}
}

// Init implements tea.Model
func (v Viewer) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (v Viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		return v, nil

	case tea.KeyMsg:
		switch v.mode {
		case ModeNormal:
			return v.updateNormal(msg)
		case ModeSearch:
			return v.updateSearch(msg)
		}
	}
	return v, nil
}

func (v Viewer) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys that work in any pane
	switch msg.String() {
	case "q", "ctrl+c":
		v.quitting = true
		return v, tea.Quit

	case "tab":
		// Switch between panes
		if v.focusPane == PaneSidebar {
			v.focusPane = PaneContent
		} else {
			v.focusPane = PaneSidebar
		}
		return v, nil

	case "/":
		// Enter search mode (search all)
		v.mode = ModeSearch
		v.searchInput = ""
		v.searchType = SearchAll
		return v, nil

	case "o":
		// Enter search mode (option only, partial match)
		v.mode = ModeSearch
		v.searchInput = ""
		v.searchType = SearchOption
		return v, nil

	case "O":
		// Enter search mode (option only, exact match)
		v.mode = ModeSearch
		v.searchInput = ""
		v.searchType = SearchOptionExact
		return v, nil

	case "d":
		// Enter search mode (description only)
		v.mode = ModeSearch
		v.searchInput = ""
		v.searchType = SearchDescription
		return v, nil

	case "esc":
		// Clear search and reset sidebar filter
		v.searchQuery = ""
		v.filteredIndices = nil
		v.matchingLines = nil
		v.currentMatch = 0
		v.sidebarCursor = 0
		v.sidebarScrollOffset = 0
		return v, nil

	case "n":
		// Next match (works from any pane, focuses content)
		matchCount := v.totalMatches()
		if matchCount > 0 {
			v.currentMatch = (v.currentMatch + 1) % matchCount
			v.scrollToCurrentMatch()
			v.focusPane = PaneContent
		}
		return v, nil

	case "N":
		// Previous match (works from any pane, focuses content)
		matchCount := v.totalMatches()
		if matchCount > 0 {
			v.currentMatch--
			if v.currentMatch < 0 {
				v.currentMatch = matchCount - 1
			}
			v.scrollToCurrentMatch()
			v.focusPane = PaneContent
		}
		return v, nil
	}

	// Pane-specific keys
	if v.focusPane == PaneSidebar {
		return v.updateSidebar(msg)
	}
	return v.updateContent(msg)
}

func (v Viewer) updateSidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	displayedIndices := v.getDisplayedSectionIndices()
	if len(displayedIndices) == 0 {
		return v, nil
	}

	switch msg.String() {
	case "up", "k":
		if v.sidebarCursor > 0 {
			v.sidebarCursor--
			v.adjustSidebarScroll()
		}
		return v, nil

	case "down", "j":
		if v.sidebarCursor < len(displayedIndices)-1 {
			v.sidebarCursor++
			v.adjustSidebarScroll()
		}
		return v, nil

	case "enter", "right", "l":
		// Jump to the selected section in content
		sectionIdx := displayedIndices[v.sidebarCursor]
		section := v.content.Sections[sectionIdx]
		v.scrollOffset = section.StartLine
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if v.scrollOffset > maxScroll {
			v.scrollOffset = maxScroll
		}
		// Switch to content pane after jumping
		v.focusPane = PaneContent
		return v, nil

	case "home", "g":
		v.sidebarCursor = 0
		v.sidebarScrollOffset = 0
		return v, nil

	case "end", "G":
		v.sidebarCursor = len(displayedIndices) - 1
		v.adjustSidebarScroll()
		return v, nil
	}
	return v, nil
}

// adjustSidebarScroll ensures the sidebar cursor is visible
func (v *Viewer) adjustSidebarScroll() {
	vpHeight := v.viewportHeight()
	if v.sidebarCursor < v.sidebarScrollOffset {
		v.sidebarScrollOffset = v.sidebarCursor
	} else if v.sidebarCursor >= v.sidebarScrollOffset+vpHeight {
		v.sidebarScrollOffset = v.sidebarCursor - vpHeight + 1
	}
}

func (v Viewer) updateContent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if v.scrollOffset > 0 {
			v.scrollOffset--
		}
		return v, nil

	case "down", "j":
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if v.scrollOffset < maxScroll {
			v.scrollOffset++
		}
		return v, nil

	case "pgup", "ctrl+u":
		v.scrollOffset -= v.viewportHeight() / 2
		if v.scrollOffset < 0 {
			v.scrollOffset = 0
		}
		return v, nil

	case "pgdown", "ctrl+d":
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		v.scrollOffset += v.viewportHeight() / 2
		if v.scrollOffset > maxScroll {
			v.scrollOffset = maxScroll
		}
		return v, nil

	case "home", "g":
		v.scrollOffset = 0
		return v, nil

	case "end", "G":
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		v.scrollOffset = maxScroll
		return v, nil

	case "left", "h":
		// Switch to sidebar
		v.focusPane = PaneSidebar
		return v, nil
	}
	return v, nil
}

func (v Viewer) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		// Quit application
		v.quitting = true
		return v, tea.Quit

	case "esc":
		// Cancel search, go back to normal mode
		v.mode = ModeNormal
		v.searchInput = ""
		return v, nil

	case "enter":
		// Execute search
		v.searchQuery = v.searchInput
		v.currentMatch = 0
		// Reset sidebar cursor and scroll for filtered view
		v.sidebarCursor = 0
		v.sidebarScrollOffset = 0
		if v.searchType == SearchAll {
			// Full-text search across all lines
			v.matchingLines = v.findMatchingLines()
			v.filteredIndices = nil
			if len(v.matchingLines) > 0 {
				v.scrollToCurrentMatch()
			}
		} else {
			// Section-based search (option or description)
			v.filteredIndices = v.findMatchingSections()
			v.matchingLines = nil
			if len(v.filteredIndices) > 0 {
				v.scrollToCurrentMatch()
			}
		}
		v.mode = ModeNormal
		return v, nil

	case "backspace":
		if len(v.searchInput) > 0 {
			v.searchInput = v.searchInput[:len(v.searchInput)-1]
		}
		return v, nil

	default:
		// Add printable characters
		if len(msg.String()) == 1 {
			v.searchInput += msg.String()
		}
		return v, nil
	}
}

// findMatchingSections returns indices of sections matching the current search query
func (v Viewer) findMatchingSections() []int {
	var indices []int
	for i, section := range v.content.Sections {
		var matches bool
		switch v.searchType {
		case SearchOption:
			matches = section.MatchesOption(v.searchQuery)
		case SearchOptionExact:
			matches = section.MatchesOptionExact(v.searchQuery)
		case SearchDescription:
			matches = section.MatchesDescription(v.searchQuery)
		default:
			matches = section.MatchesQuery(v.searchQuery)
		}
		if matches {
			indices = append(indices, i)
		}
	}
	return indices
}

// totalMatches returns the total number of search matches
func (v Viewer) totalMatches() int {
	if len(v.matchingLines) > 0 {
		return len(v.matchingLines)
	}
	return len(v.filteredIndices)
}

// findMatchingLines returns line numbers where the search query appears (for full-text search)
func (v Viewer) findMatchingLines() []int {
	var lineNums []int
	query := strings.ToLower(v.searchQuery)
	for i, line := range v.content.Lines {
		if strings.Contains(strings.ToLower(line), query) {
			lineNums = append(lineNums, i)
		}
	}
	return lineNums
}

// scrollToCurrentMatch scrolls the viewport to show the current match
func (v *Viewer) scrollToCurrentMatch() {
	var targetLine int

	if len(v.matchingLines) > 0 {
		// Line-based search (full-text)
		targetLine = v.matchingLines[v.currentMatch]
	} else if len(v.filteredIndices) > 0 {
		// Section-based search
		sectionIdx := v.filteredIndices[v.currentMatch]
		section := v.content.Sections[sectionIdx]
		targetLine = section.StartLine
		// Also update sidebar cursor to match the current section
		v.sidebarCursor = sectionIdx
		v.adjustSidebarScroll()
	} else {
		return
	}

	// Scroll to center the target line in viewport
	v.scrollOffset = targetLine - v.viewportHeight()/2
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
	maxScroll := len(v.content.Lines) - v.viewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if v.scrollOffset > maxScroll {
		v.scrollOffset = maxScroll
	}
}

// viewportHeight returns the height available for content (minus status lines)
func (v Viewer) viewportHeight() int {
	// Reserve 3 lines: 1 for title, 1 for command line, 1 for help
	return v.height - 3
}

// isLineMatching checks if a line number is a match (either direct line match or within matching section)
func (v Viewer) isLineMatching(lineNum int) bool {
	// Check line-based matches first
	for _, matchLine := range v.matchingLines {
		if lineNum == matchLine {
			return true
		}
	}
	// Check section-based matches
	for _, idx := range v.filteredIndices {
		section := v.content.Sections[idx]
		if lineNum >= section.StartLine && lineNum <= section.EndLine {
			return true
		}
	}
	return false
}

// sidebarWidth returns the width of the sidebar
func (v Viewer) sidebarWidth() int {
	return 30
}

// getDisplayedSectionIndices returns the indices of sections to display in the sidebar.
// When a search is active, only matching sections are shown.
func (v Viewer) getDisplayedSectionIndices() []int {
	if v.searchQuery == "" {
		// No search active, show all sections
		indices := make([]int, len(v.content.Sections))
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	// Section-based search (option or description)
	if len(v.filteredIndices) > 0 {
		return v.filteredIndices
	}

	// Full-text search - find sections that contain the search query
	if len(v.matchingLines) > 0 {
		var indices []int
		for i, section := range v.content.Sections {
			if section.MatchesQuery(v.searchQuery) {
				indices = append(indices, i)
			}
		}
		return indices
	}

	// No matches, show empty
	return nil
}

// contentWidth returns the width of the content pane
func (v Viewer) contentWidth() int {
	return v.width - v.sidebarWidth() - 1 // -1 for border
}

// truncateOption truncates an option string to fit in the sidebar
func truncateOption(opt string, maxWidth int) string {
	if len(opt) <= maxWidth {
		return opt
	}
	if maxWidth <= 3 {
		return opt[:maxWidth]
	}
	return opt[:maxWidth-3] + "..."
}

// renderSidebar renders the left sidebar with section list
func (v Viewer) renderSidebar() string {
	var b strings.Builder
	sidebarW := v.sidebarWidth()
	vpHeight := v.viewportHeight()

	// Sidebar styles
	var borderColor lipgloss.Color
	if v.focusPane == PaneSidebar {
		borderColor = lipgloss.Color("212") // Pink when focused
	} else {
		borderColor = lipgloss.Color("241") // Gray when not focused
	}

	sidebarSelectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(sidebarW - 2)

	sidebarNormalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(sidebarW - 2)

	displayedIndices := v.getDisplayedSectionIndices()

	for i := 0; i < vpHeight; i++ {
		displayIdx := v.sidebarScrollOffset + i
		var line string
		if displayIdx < len(displayedIndices) {
			sectionIdx := displayedIndices[displayIdx]
			opt := truncateOption(v.content.Sections[sectionIdx].Option, sidebarW-4)
			if displayIdx == v.sidebarCursor {
				line = sidebarSelectedStyle.Render("> " + opt)
			} else {
				line = sidebarNormalStyle.Render("  " + opt)
			}
		} else {
			line = sidebarNormalStyle.Render("")
		}
		b.WriteString(line)
		if i < vpHeight-1 {
			b.WriteString("\n")
		}
	}

	// Wrap sidebar content in a border
	sidebarStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderRight(true).
		Width(sidebarW)

	return sidebarStyle.Render(b.String())
}

// highlightSearchTerm highlights occurrences of the search query in a line
func (v Viewer) highlightSearchTerm(line string) string {
	if v.searchQuery == "" {
		return line
	}

	// Only highlight search term for full-text search (SearchAll)
	// For option/description searches, we filter sections but don't highlight the term in content
	if v.searchType != SearchAll {
		return line
	}

	// Style for the search term itself - bright yellow on dark red for maximum visibility
	termStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("208")). // Bright orange background
		Foreground(lipgloss.Color("0")).   // Black text
		Bold(true)

	// Case-insensitive search and replace
	lowerLine := strings.ToLower(line)
	lowerQuery := strings.ToLower(v.searchQuery)

	var result strings.Builder
	lastEnd := 0

	for {
		idx := strings.Index(lowerLine[lastEnd:], lowerQuery)
		if idx == -1 {
			// No more matches, append the rest
			result.WriteString(line[lastEnd:])
			break
		}

		// Append text before the match
		matchStart := lastEnd + idx
		result.WriteString(line[lastEnd:matchStart])

		// Append the highlighted match (preserve original case)
		matchEnd := matchStart + len(v.searchQuery)
		result.WriteString(termStyle.Render(line[matchStart:matchEnd]))

		lastEnd = matchEnd
	}

	return result.String()
}

// renderContent renders the right content pane
func (v Viewer) renderContent() string {
	var b strings.Builder
	vpHeight := v.viewportHeight()
	contentW := v.contentWidth()

	// Style for matching lines (subtle green background)
	matchingLineStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("22")).
		Foreground(lipgloss.Color("252"))

	normalLineStyle := lipgloss.NewStyle()

	for i := 0; i < vpHeight; i++ {
		lineIdx := v.scrollOffset + i
		var line string
		if lineIdx < len(v.content.Lines) {
			line = v.content.Lines[lineIdx]
			// Truncate if too long
			if len(line) > contentW {
				line = line[:contentW]
			}
		}

		// Highlight matching lines and search terms
		if v.searchQuery != "" && v.isLineMatching(lineIdx) {
			// First highlight the search term, then apply line background
			highlightedLine := v.highlightSearchTerm(line)
			// Pad to contentW for consistent background
			padding := contentW - len(line)
			if padding > 0 {
				highlightedLine += strings.Repeat(" ", padding)
			}
			b.WriteString(matchingLineStyle.Render(highlightedLine))
		} else {
			// Pad to contentW
			padding := contentW - len(line)
			if padding > 0 {
				line += strings.Repeat(" ", padding)
			}
			b.WriteString(normalLineStyle.Render(line))
		}
		if i < vpHeight-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// View implements tea.Model
func (v Viewer) View() string {
	if v.quitting {
		return ""
	}

	var b strings.Builder

	// Title bar
	title := fmt.Sprintf(" %s(%s) ", v.manPage.Name, v.manPage.Section)
	if v.searchQuery != "" {
		matchCount := v.totalMatches()
		matchInfo := fmt.Sprintf(" [%d/%d matches] ", v.currentMatch+1, matchCount)
		if matchCount == 0 {
			matchInfo = " [no matches] "
		}
		var searchPrefix string
		switch v.searchType {
		case SearchOption:
			searchPrefix = "option:"
		case SearchOptionExact:
			searchPrefix = "option(exact):"
		case SearchDescription:
			searchPrefix = "desc:"
		default:
			searchPrefix = "search:"
		}
		title += searchPrefix + " " + v.searchQuery + matchInfo
	}
	titleBar := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(v.width).
		Render(title)
	b.WriteString(titleBar)
	b.WriteString("\n")

	// Two-column layout: sidebar + content
	sidebar := v.renderSidebar()
	content := v.renderContent()

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
	b.WriteString(mainArea)
	b.WriteString("\n")

	// Command line / status bar
	var cmdLine string
	switch v.mode {
	case ModeSearch:
		var prefix string
		switch v.searchType {
		case SearchOption:
			prefix = "o:"
		case SearchOptionExact:
			prefix = "O:"
		case SearchDescription:
			prefix = "d:"
		default:
			prefix = "/"
		}
		cmdLine = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Render(prefix) + v.searchInput + "█"
	case ModeNormal:
		if v.searchQuery != "" {
			cmdLine = helpStyle.Render("n next • N prev • esc clear • tab switch • / search • q quit")
		} else {
			cmdLine = helpStyle.Render("tab switch • ↑↓ navigate • enter select • /oOd search • q quit")
		}
	}
	cmdLineBar := lipgloss.NewStyle().
		Width(v.width).
		Render(cmdLine)
	b.WriteString(cmdLineBar)

	return b.String()
}
