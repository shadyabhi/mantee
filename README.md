# mantee

A TUI man page viewer with search and navigation.

## Installation

### Homebrew

```bash
brew install shadyabhi/tap/mantee
```

### Go

```bash
go install github.com/shadyabhi/mantee@latest
```

## Usage

```bash
mantee          # Interactive search prompt
mantee grep     # Search for "grep" and select from results
```

## Keybindings

### Navigation
- `Tab` / `Shift+Tab` - Cycle between panes (Options, Content, Sections)
- `j/k` or `↑/↓` - Navigate within pane
- `Enter` - Select item / jump to section
- `g` - Open section selector modal

### Search
- `/` - Full-text search
- `o` - Search options (partial match)
- `O` - Search options (exact match)
- `d` - Search descriptions
- `n/N` - Next/previous match
- `Esc` - Clear search

### General
- `q` - Quit

## License

MIT
