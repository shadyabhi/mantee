package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shadyabhi/mantee/man/parse"
	"github.com/shadyabhi/mantee/man/search"
	searchui "github.com/shadyabhi/mantee/search"
	"github.com/shadyabhi/mantee/viewer"
)

// Run orchestrates the two-stage UI flow: search/selection → viewer
func Run(keyword string) error {
	var model searchui.Model

	if keyword != "" {
		// Keyword provided - search and go directly to selection
		pages, err := search.SearchManPages(keyword)
		if err != nil {
			return fmt.Errorf("searching man pages: %w", err)
		}

		if len(pages) == 0 {
			return fmt.Errorf("no man pages found for: %s", keyword)
		}

		model = searchui.NewWithResults(keyword, pages)
	} else {
		// No keyword - start with text input
		model = searchui.New()
	}

	// Run the search/selection UI
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("running search UI: %w", err)
	}

	// Check if a page was selected
	m := finalModel.(searchui.Model)
	selected := m.Selected()
	if selected == nil {
		// User quit without selecting
		return nil
	}

	// Fetch the man page content
	content, err := parse.FetchManPage(selected.Section, selected.Name)
	if err != nil {
		return fmt.Errorf("fetching man page: %w", err)
	}

	// Launch the viewer
	v := viewer.New(*selected, content)
	viewerProgram := tea.NewProgram(v, tea.WithAltScreen(), tea.WithMouseCellMotion())

	_, err = viewerProgram.Run()
	if err != nil {
		return fmt.Errorf("running viewer: %w", err)
	}

	return nil
}
