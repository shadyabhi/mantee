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
	ModeNormal        ViewerMode = iota // Normal viewing mode
	ModeSearch                          // Search/command input mode
	ModeSectionSelect                   // Section selector modal
	ModeHelp                            // Help/shortcuts modal
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
	PaneSidebar  FocusPane = iota // Left sidebar with options list
	PaneContent                   // Center content pane
	PaneSections                  // Right sidebar with man sections
	PaneCount                     // Total number of panes (must be last)
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
	contentCursor       int        // Cursor position within content (relative to scrollOffset)
	width               int
	height              int
	quitting            bool
	// Section selector state
	sectionCursor       int // Current selection in section selector modal
	sectionScrollOffset int // Scroll offset for section selector
}

// NewViewer creates a new Viewer for the given man page
func NewViewer(page ManPage, content *ManPageContent) Viewer {
	return Viewer{
		content:   content,
		manPage:   page,
		mode:      ModeNormal,
		focusPane: PaneContent,
		width:     80,
		height:    24,
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
		case ModeSectionSelect:
			return v.updateSectionSelect(msg)
		case ModeHelp:
			return v.updateHelp(msg)
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
		// Cycle through panes forward
		v.focusPane = (v.focusPane + 1) % PaneCount
		return v, nil

	case "shift+tab":
		// Cycle through panes backward
		v.focusPane = (v.focusPane + PaneCount - 1) % PaneCount
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

	case "G":
		// Open section selector modal
		if len(v.content.ManSections) > 0 {
			v.mode = ModeSectionSelect
			v.sectionCursor = 0
			v.sectionScrollOffset = 0
		}
		return v, nil

	case "?":
		// Open help modal
		v.mode = ModeHelp
		return v, nil
	}

	// Pane-specific keys
	switch v.focusPane {
	case PaneSidebar:
		return v.updateSidebar(msg)
	case PaneSections:
		return v.updateSections(msg)
	default:
		return v.updateContent(msg)
	}
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

	case "home":
		v.sidebarCursor = 0
		v.sidebarScrollOffset = 0
		return v, nil

	case "G":
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
	vpHeight := v.viewportHeight()
	maxLine := len(v.content.Lines) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	switch msg.String() {
	case "up", "k":
		// Move cursor up
		currentLine := v.scrollOffset + v.contentCursor
		if currentLine > 0 {
			if v.contentCursor > 0 {
				// Cursor can move up within viewport
				v.contentCursor--
			} else {
				// Cursor at top of viewport, scroll up
				v.scrollOffset--
			}
		}
		return v, nil

	case "down", "j":
		// Move cursor down
		currentLine := v.scrollOffset + v.contentCursor
		if currentLine < maxLine {
			if v.contentCursor < vpHeight-1 {
				// Cursor can move down within viewport
				v.contentCursor++
			} else {
				// Cursor at bottom of viewport, scroll down
				v.scrollOffset++
			}
		}
		return v, nil

	case "pgup", "ctrl+u":
		halfPage := vpHeight / 2
		currentLine := v.scrollOffset + v.contentCursor
		newLine := currentLine - halfPage
		if newLine < 0 {
			newLine = 0
		}
		// Adjust scroll and cursor
		if newLine < v.scrollOffset {
			v.scrollOffset = newLine
			v.contentCursor = 0
		} else {
			v.contentCursor = newLine - v.scrollOffset
		}
		return v, nil

	case "pgdown", "ctrl+d":
		halfPage := vpHeight / 2
		currentLine := v.scrollOffset + v.contentCursor
		newLine := currentLine + halfPage
		if newLine > maxLine {
			newLine = maxLine
		}
		// Adjust scroll and cursor
		maxScroll := len(v.content.Lines) - vpHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if newLine >= v.scrollOffset+vpHeight {
			v.scrollOffset = newLine - vpHeight + 1
			if v.scrollOffset > maxScroll {
				v.scrollOffset = maxScroll
			}
			v.contentCursor = newLine - v.scrollOffset
		} else {
			v.contentCursor = newLine - v.scrollOffset
		}
		return v, nil

	case "home":
		v.scrollOffset = 0
		v.contentCursor = 0
		return v, nil

	case "G":
		maxScroll := len(v.content.Lines) - vpHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		v.scrollOffset = maxScroll
		v.contentCursor = vpHeight - 1
		if v.contentCursor > maxLine-v.scrollOffset {
			v.contentCursor = maxLine - v.scrollOffset
		}
		return v, nil

	case "left", "h":
		// Switch to sidebar
		v.focusPane = PaneSidebar
		return v, nil

	case "right", "l":
		// Switch to sections pane
		v.focusPane = PaneSections
		return v, nil
	}
	return v, nil
}

// updateSections handles key events for the sections pane (right sidebar)
func (v Viewer) updateSections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sections := v.content.ManSections
	if len(sections) == 0 {
		return v, nil
	}

	// Use sectionCursor for navigation in sections pane
	switch msg.String() {
	case "up", "k":
		if v.sectionCursor > 0 {
			v.sectionCursor--
		}
		return v, nil

	case "down", "j":
		if v.sectionCursor < len(sections)-1 {
			v.sectionCursor++
		}
		return v, nil

	case "enter", "l":
		// Jump to selected section
		section := sections[v.sectionCursor]
		v.scrollOffset = section.StartLine
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if v.scrollOffset > maxScroll {
			v.scrollOffset = maxScroll
		}
		v.focusPane = PaneContent
		return v, nil

	case "home":
		v.sectionCursor = 0
		return v, nil

	case "G":
		v.sectionCursor = len(sections) - 1
		return v, nil

	case "left", "h":
		// Switch to content pane
		v.focusPane = PaneContent
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
		v.focusPane = PaneContent // Keep focus on content pane after search
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

func (v Viewer) updateSectionSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sections := v.content.ManSections
	if len(sections) == 0 {
		v.mode = ModeNormal
		return v, nil
	}

	switch msg.String() {
	case "ctrl+c":
		v.quitting = true
		return v, tea.Quit

	case "esc", "g":
		// Close section selector
		v.mode = ModeNormal
		return v, nil

	case "up", "k":
		if v.sectionCursor > 0 {
			v.sectionCursor--
			v.adjustSectionScroll()
		}
		return v, nil

	case "down", "j":
		if v.sectionCursor < len(sections)-1 {
			v.sectionCursor++
			v.adjustSectionScroll()
		}
		return v, nil

	case "enter", "l":
		// Jump to selected section
		section := sections[v.sectionCursor]
		v.scrollOffset = section.StartLine
		maxScroll := len(v.content.Lines) - v.viewportHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if v.scrollOffset > maxScroll {
			v.scrollOffset = maxScroll
		}
		v.mode = ModeNormal
		v.focusPane = PaneContent
		return v, nil

	case "home":
		v.sectionCursor = 0
		v.sectionScrollOffset = 0
		return v, nil

	case "end", "G":
		v.sectionCursor = len(sections) - 1
		v.adjustSectionScroll()
		return v, nil
	}
	return v, nil
}

func (v Viewer) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		v.quitting = true
		return v, tea.Quit

	case "esc", "?", "q":
		// Close help modal
		v.mode = ModeNormal
		return v, nil
	}
	return v, nil
}

// adjustSectionScroll ensures the section cursor is visible in the modal
func (v *Viewer) adjustSectionScroll() {
	modalHeight := v.sectionModalHeight()
	if v.sectionCursor < v.sectionScrollOffset {
		v.sectionScrollOffset = v.sectionCursor
	} else if v.sectionCursor >= v.sectionScrollOffset+modalHeight {
		v.sectionScrollOffset = v.sectionCursor - modalHeight + 1
	}
}

// sectionModalHeight returns the number of visible items in the section modal
func (v Viewer) sectionModalHeight() int {
	// Modal takes about half the screen height, minus borders
	maxHeight := v.height/2 - 4
	if maxHeight < 5 {
		maxHeight = 5
	}
	numSections := len(v.content.ManSections)
	if numSections < maxHeight {
		return numSections
	}
	return maxHeight
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

// isCurrentMatchLine checks if a line number is the currently selected match
func (v Viewer) isCurrentMatchLine(lineNum int) bool {
	if len(v.matchingLines) > 0 {
		// Line-based search (full-text)
		return lineNum == v.matchingLines[v.currentMatch]
	}
	if len(v.filteredIndices) > 0 {
		// Section-based search - current match is the start line of the section
		sectionIdx := v.filteredIndices[v.currentMatch]
		section := v.content.Sections[sectionIdx]
		return lineNum == section.StartLine
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

// sectionsPaneWidth returns the width of the right sections pane
func (v Viewer) sectionsPaneWidth() int {
	return 22
}

// calculatePercentage returns the percentage position (0-100) given current position and total items
func calculatePercentage(current, total int) int {
	if total <= 1 {
		return 100
	}
	return current * 100 / (total - 1)
}

// currentManSectionIndex returns the index of the man section currently visible
// based on the cursor position (returns -1 if no sections)
func (v Viewer) currentManSectionIndex() int {
	sections := v.content.ManSections
	if len(sections) == 0 {
		return -1
	}

	// Find which section contains the current cursor position
	currentLine := v.scrollOffset + v.contentCursor
	currentIdx := 0

	for i, section := range sections {
		if section.StartLine <= currentLine {
			currentIdx = i
		} else {
			break
		}
	}

	return currentIdx
}

// contentWidth returns the width of the content pane
func (v Viewer) contentWidth() int {
	return v.width - v.sidebarWidth() - v.sectionsPaneWidth() - 2 // -2 for borders
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
	vpHeight := v.viewportHeight() - 1 // -1 for title

	// Sidebar styles
	var borderColor, titleBg lipgloss.Color
	if v.focusPane == PaneSidebar {
		borderColor = lipgloss.Color("212") // Pink when focused
		titleBg = lipgloss.Color("62")      // Brighter purple when focused
	} else {
		borderColor = lipgloss.Color("241") // Gray when not focused
		titleBg = lipgloss.Color("238")     // Dark gray when not focused
	}

	// Title bar with percentage completion
	displayedIndices := v.getDisplayedSectionIndices()
	percentage := calculatePercentage(v.sidebarCursor, len(displayedIndices))
	titleText := fmt.Sprintf("OPTIONS (%d%%)", percentage)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(titleBg).
		Width(sidebarW - 2).
		Align(lipgloss.Center)
	b.WriteString(titleStyle.Render(titleText))
	b.WriteString("\n")

	sidebarSelectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(sidebarW - 2)

	sidebarNormalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(sidebarW - 2)

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
	vpHeight := v.viewportHeight() - 1 // -1 for title
	contentW := v.contentWidth() - 2   // Account for border

	// Content pane border and title color based on focus
	var borderColor, titleBg lipgloss.Color
	if v.focusPane == PaneContent {
		borderColor = lipgloss.Color("212") // Pink when focused
		titleBg = lipgloss.Color("62")      // Brighter purple when focused
	} else {
		borderColor = lipgloss.Color("241") // Gray when not focused
		titleBg = lipgloss.Color("238")     // Dark gray when not focused
	}

	// Title bar with percentage completion
	currentLine := v.scrollOffset + v.contentCursor
	percentage := calculatePercentage(currentLine, len(v.content.Lines))
	titleText := fmt.Sprintf("CONTENT (%d%%)", percentage)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(titleBg).
		Width(contentW).
		Align(lipgloss.Center)
	b.WriteString(titleStyle.Render(titleText))
	b.WriteString("\n")

	// Style for the current match (the one we navigated to with n/N)
	currentMatchStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("208")). // Bright orange background
		Foreground(lipgloss.Color("0")).   // Black text
		Bold(true)

	// Style for other matching lines (subtle green background)
	matchingLineStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("22")).
		Foreground(lipgloss.Color("252"))

	// Style for current line when content pane is focused (subtle underline effect)
	currentLineStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")). // Dark gray background
		Foreground(lipgloss.Color("255"))  // Bright white text

	normalLineStyle := lipgloss.NewStyle()

	// Arrow indicator for current match
	arrowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")). // Bright orange
		Bold(true)

	for i := 0; i < vpHeight; i++ {
		lineIdx := v.scrollOffset + i
		var line string
		if lineIdx < len(v.content.Lines) {
			line = v.content.Lines[lineIdx]
			// Truncate if too long (leave room for arrow indicator)
			maxLen := contentW - 2 // Reserve 2 chars for "→ " prefix
			if len(line) > maxLen {
				line = line[:maxLen]
			}
		}

		// Highlight matching lines and search terms
		if v.searchQuery != "" && v.isCurrentMatchLine(lineIdx) {
			// This is the CURRENT match - use distinct highlighting with arrow
			highlightedLine := v.highlightSearchTerm(line)
			padding := contentW - 2 - len(line) // -2 for arrow prefix
			if padding > 0 {
				highlightedLine += strings.Repeat(" ", padding)
			}
			b.WriteString(arrowStyle.Render("→ ") + currentMatchStyle.Render(highlightedLine))
		} else if v.searchQuery != "" && v.isLineMatching(lineIdx) {
			// Other matching lines
			highlightedLine := v.highlightSearchTerm(line)
			padding := contentW - 2 - len(line)
			if padding > 0 {
				highlightedLine += strings.Repeat(" ", padding)
			}
			b.WriteString("  " + matchingLineStyle.Render(highlightedLine))
		} else if v.focusPane == PaneContent && i == v.contentCursor {
			// Highlight the cursor line when content pane is focused
			padding := contentW - 2 - len(line)
			if padding > 0 {
				line += strings.Repeat(" ", padding)
			}
			b.WriteString("  " + currentLineStyle.Render(line))
		} else {
			// Pad to contentW
			padding := contentW - 2 - len(line)
			if padding > 0 {
				line += strings.Repeat(" ", padding)
			}
			b.WriteString("  " + normalLineStyle.Render(line))
		}
		if i < vpHeight-1 {
			b.WriteString("\n")
		}
	}

	// Wrap content in a border
	contentStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderLeft(true).
		Width(v.contentWidth())

	return contentStyle.Render(b.String())
}

// renderSectionsPane renders the right sidebar with man page sections
func (v Viewer) renderSectionsPane() string {
	var b strings.Builder
	paneW := v.sectionsPaneWidth()
	vpHeight := v.viewportHeight() - 1 // -1 for title
	sections := v.content.ManSections

	// When focused, use sectionCursor; otherwise show current viewing section
	var highlightIdx int
	if v.focusPane == PaneSections {
		highlightIdx = v.sectionCursor
	} else {
		highlightIdx = v.currentManSectionIndex()
	}

	// Sections pane border and title color based on focus
	var borderColor, titleBg lipgloss.Color
	if v.focusPane == PaneSections {
		borderColor = lipgloss.Color("212") // Pink when focused
		titleBg = lipgloss.Color("62")      // Brighter purple when focused
	} else {
		borderColor = lipgloss.Color("241") // Gray when not focused
		titleBg = lipgloss.Color("238")     // Dark gray when not focused
	}

	// Title bar with percentage completion
	percentage := calculatePercentage(highlightIdx, len(sections))
	titleText := fmt.Sprintf("SECTIONS (%d%%)", percentage)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(titleBg).
		Width(paneW - 4).
		Align(lipgloss.Center)
	b.WriteString(titleStyle.Render(titleText))
	b.WriteString("\n")

	// Styles for section items
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(paneW - 4)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(paneW - 4)

	for i := 0; i < vpHeight; i++ {
		var line string
		if i < len(sections) {
			section := sections[i]
			name := section.Name
			if len(name) > paneW-6 {
				name = name[:paneW-9] + "..."
			}
			if i == highlightIdx {
				line = selectedStyle.Render("> " + name)
			} else {
				line = normalStyle.Render("  " + name)
			}
		} else {
			line = normalStyle.Render("")
		}
		b.WriteString(line)
		if i < vpHeight-1 {
			b.WriteString("\n")
		}
	}

	// Wrap in a border
	paneStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderLeft(true).
		Width(paneW)

	return paneStyle.Render(b.String())
}

// renderSectionModal renders the section selector modal overlay
func (v Viewer) renderSectionModal() string {
	sections := v.content.ManSections
	if len(sections) == 0 {
		return ""
	}

	modalHeight := v.sectionModalHeight()
	modalWidth := 40

	// Modal styles
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(modalWidth - 4)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(modalWidth - 4)

	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	lines = append(lines, titleStyle.Render("Go to Section"))
	lines = append(lines, strings.Repeat("─", modalWidth-4))

	// Section list
	for i := 0; i < modalHeight; i++ {
		idx := v.sectionScrollOffset + i
		if idx >= len(sections) {
			lines = append(lines, normalStyle.Render(""))
			continue
		}
		section := sections[idx]
		var line string
		if idx == v.sectionCursor {
			line = selectedStyle.Render("> " + section.Name)
		} else {
			line = normalStyle.Render("  " + section.Name)
		}
		lines = append(lines, line)
	}

	// Help line
	lines = append(lines, strings.Repeat("─", modalWidth-4))
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	lines = append(lines, helpStyle.Render("↑↓ navigate • enter select • esc close"))

	content := strings.Join(lines, "\n")

	// Modal box with border
	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("212")).
		Padding(0, 1).
		Width(modalWidth)

	return modalStyle.Render(content)
}

// renderHelpModal renders the help/shortcuts modal overlay
func (v Viewer) renderHelpModal() string {
	modalWidth := 50

	// Define all shortcuts
	shortcuts := []struct {
		key  string
		desc string
	}{
		{"Navigation", ""},
		{"↑/k, ↓/j", "Move up/down"},
		{"←/h, →/l", "Switch panes"},
		{"tab", "Cycle panes forward"},
		{"shift+tab", "Cycle panes backward"},
		{"pgup/ctrl+u", "Page up"},
		{"pgdown/ctrl+d", "Page down"},
		{"home", "Go to top"},
		{"G", "Go to bottom / Open sections"},
		{"enter", "Select item / Jump to section"},
		{"", ""},
		{"Search", ""},
		{"/", "Search all content"},
		{"o", "Search options (partial)"},
		{"O", "Search options (exact)"},
		{"d", "Search descriptions"},
		{"n", "Next match"},
		{"N", "Previous match"},
		{"esc", "Clear search"},
		{"", ""},
		{"Other", ""},
		{"?", "Show this help"},
		{"q", "Quit"},
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Width(modalWidth - 4).
		Align(lipgloss.Center)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Width(modalWidth - 4)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("Keyboard Shortcuts"))
	lines = append(lines, strings.Repeat("─", modalWidth-4))

	// Shortcuts list
	for _, s := range shortcuts {
		if s.key == "" && s.desc == "" {
			// Empty line
			lines = append(lines, "")
		} else if s.desc == "" {
			// Section header
			lines = append(lines, headerStyle.Render(s.key))
		} else {
			// Key-description pair
			keyPart := keyStyle.Render(fmt.Sprintf("%-14s", s.key))
			descPart := descStyle.Render(s.desc)
			lines = append(lines, "  "+keyPart+" "+descPart)
		}
	}

	// Help line
	lines = append(lines, strings.Repeat("─", modalWidth-4))
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	lines = append(lines, footerStyle.Render("Press ?, esc, or q to close"))

	content := strings.Join(lines, "\n")

	// Modal box with border
	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("212")).
		Padding(0, 1).
		Width(modalWidth)

	return modalStyle.Render(content)
}

// overlayModal centers the modal over the background content
func (v Viewer) overlayModal(background, modal string) string {
	bgLines := strings.Split(background, "\n")
	modalLines := strings.Split(modal, "\n")

	// Calculate modal position (center)
	modalHeight := len(modalLines)
	modalWidth := 0
	for _, line := range modalLines {
		if len(line) > modalWidth {
			modalWidth = len(line)
		}
	}

	startY := (len(bgLines) - modalHeight) / 2
	startX := (v.width - modalWidth) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Overlay modal on background
	for i, modalLine := range modalLines {
		bgIdx := startY + i
		if bgIdx >= len(bgLines) {
			break
		}

		bgLine := bgLines[bgIdx]
		// Convert to runes for proper unicode handling
		bgRunes := []rune(bgLine)
		modalRunes := []rune(modalLine)

		// Pad background line if needed
		for len(bgRunes) < startX+len(modalRunes) {
			bgRunes = append(bgRunes, ' ')
		}

		// Overlay modal characters
		for j, r := range modalRunes {
			if startX+j < len(bgRunes) {
				bgRunes[startX+j] = r
			}
		}

		bgLines[bgIdx] = string(bgRunes)
	}

	return strings.Join(bgLines, "\n")
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

	// Three-column layout: sidebar + content + sections pane
	sidebar := v.renderSidebar()
	content := v.renderContent()
	sectionsPane := v.renderSectionsPane()

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content, sectionsPane)

	// Overlay modal if in section select or help mode
	if v.mode == ModeSectionSelect {
		modal := v.renderSectionModal()
		mainArea = v.overlayModal(mainArea, modal)
	} else if v.mode == ModeHelp {
		modal := v.renderHelpModal()
		mainArea = v.overlayModal(mainArea, modal)
	}

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
			cmdLine = helpStyle.Render("n next • N prev • esc clear • tab switch • G sections • ? help • q quit")
		} else {
			cmdLine = helpStyle.Render("tab switch • ↑↓ navigate • enter select • G sections • ? help • q quit")
		}
	case ModeSectionSelect:
		cmdLine = helpStyle.Render("↑↓ navigate • enter jump • esc/G close")
	case ModeHelp:
		cmdLine = helpStyle.Render("Press ?, esc, or q to close")
	}
	cmdLineBar := lipgloss.NewStyle().
		Width(v.width).
		Render(cmdLine)
	b.WriteString(cmdLineBar)

	return b.String()
}
