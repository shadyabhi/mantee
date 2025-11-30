package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var model Model

	if len(os.Args) >= 2 {
		// Keyword provided via CLI - search and go directly to selection
		keyword := os.Args[1]

		pages, err := SearchManPages(keyword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error searching man pages: %v\n", err)
			os.Exit(1)
		}

		if len(pages) == 0 {
			fmt.Fprintf(os.Stderr, "No man pages found for: %s\n", keyword)
			os.Exit(1)
		}

		model = NewSelectModel(keyword, pages)
	} else {
		// No keyword - start with text input
		model = NewInputModel()
	}

	// Run the UI
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running UI: %v\n", err)
		os.Exit(1)
	}

	// Check if a page was selected
	m := finalModel.(Model)
	selected := m.Selected()
	if selected == nil {
		// User quit without selecting
		os.Exit(0)
	}

	// Fetch the man page content
	content, err := FetchManPage(selected.Section, selected.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching man page: %v\n", err)
		os.Exit(1)
	}

	// Launch the viewer
	viewer := NewViewer(*selected, content)
	viewerProgram := tea.NewProgram(viewer, tea.WithAltScreen(), tea.WithMouseCellMotion())

	_, err = viewerProgram.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running viewer: %v\n", err)
		os.Exit(1)
	}
}
