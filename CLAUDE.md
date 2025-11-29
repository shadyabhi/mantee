# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run

```bash
go build              # Build the binary
./mantee              # Run with interactive search prompt
./mantee grep         # Search for "grep" and select from results
```

## Architecture

mantee is a TUI man page viewer built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-style TUI framework) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) (styling).

### File Structure

- **main.go** - Entry point, launches either input mode or selection mode based on CLI args
- **ui.go** - Initial UI model (`Model`) for search input and man page selection list
- **viewer.go** - Main viewer model (`Viewer`) with three-pane layout for viewing man page content
- **search.go** - Man page search via `man -k`, parsing output into `ManPage` structs
- **section.go** - Man page content fetching and parsing into sections (`Section`, `ManSection`)

### Key Types

- `Model` (ui.go) - Handles initial search input and result selection, transitions to Viewer
- `Viewer` (viewer.go) - Main viewing interface with sidebar (options), content pane, and sections pane
- `ManPage` (search.go) - Search result entry (name, section, description)
- `ManPageContent` (section.go) - Parsed man page with lines, option sections, and major sections
- `Section` (section.go) - CLI option definition extracted from man page (option flags + explanation)
- `ManSection` (section.go) - Major man page section header (NAME, SYNOPSIS, DESCRIPTION, etc.)

### UI Flow

1. User enters search term → `SearchManPages()` runs `man -k` → returns `[]ManPage`
2. User selects a man page → `FetchManPage()` runs `man` with `col -b` → returns `*ManPageContent`
3. Viewer displays three-pane layout with parsed options sidebar, content, and sections navigation

### Viewer Modes

- `ModeNormal` - Navigation mode (scroll, pane switching)
- `ModeSearch` - Text input for searching content
- `ModeSectionSelect` - Modal overlay for jumping to man page sections

### Focus Panes

- `PaneSidebar` - Left pane showing extracted CLI options
- `PaneContent` - Center pane showing man page content
- `PaneSections` - Right pane showing major sections (NAME, DESCRIPTION, etc.)
