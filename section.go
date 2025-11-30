package main

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// Maximum length for a line containing just option flags
	// Description text lines are typically much longer
	maxOptionLineLength = 60
)

// Section represents a CLI option section from a man page
type Section struct {
	Option      string // The CLI option(s), e.g., "-r, --recursive"
	Explanation string // The explanation text for this option
	StartLine   int    // Line number where this section starts in the raw content
	EndLine     int    // Line number where this section ends
}

// ManSection represents a major section in a man page (NAME, SYNOPSIS, DESCRIPTION, etc.)
type ManSection struct {
	Name      string // The section name, e.g., "NAME", "SYNOPSIS", "DESCRIPTION"
	StartLine int    // Line number where this section starts
}

// ManPageContent represents the full content of a man page
type ManPageContent struct {
	RawContent  string       // The full man page text
	Lines       []string     // Lines of the man page
	Sections    []Section    // Parsed option sections
	ManSections []ManSection // Major man page sections (NAME, SYNOPSIS, etc.)
}

// FetchManPage retrieves the content of a man page
func FetchManPage(section, name string) (*ManPageContent, error) {
	// Use MANWIDTH to control line width, and col -b to strip formatting
	cmd := exec.Command("sh", "-c", "MANWIDTH=80 man "+section+" "+name+" | col -b")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	content := stdout.String()
	lines := strings.Split(content, "\n")

	mpc := &ManPageContent{
		RawContent:  content,
		Lines:       lines,
		Sections:    parseOptionSections(lines),
		ManSections: parseManSections(lines),
	}

	return mpc, nil
}

// parseOptionSections extracts option sections from man page lines
// Scans the entire man page for option definitions
func parseOptionSections(lines []string) []Section {
	var sections []Section

	// Option definition pattern: line starting with specific indentation (typically 5-8 spaces)
	// followed by a dash and option name.
	// Pattern: 5-8 spaces, then -X (any char) or --word
	// Using \S to match any non-whitespace for single-char options (handles -@, -%, etc.)
	optionDefRe := regexp.MustCompile(`^(\s{5,8})(-\S|--[a-zA-Z][-a-zA-Z0-9]*)`)

	// Pattern to detect lines that are lists of multiple --long options (not definitions)
	// e.g., "--show-error, --stderr, --styled-output, --trace-ascii,"
	multiOptionListRe := regexp.MustCompile(`--\w+,\s+--\w+,\s+--\w+`)

	// Regex to match major section headers (all caps at start of line)
	sectionHeaderRe := regexp.MustCompile(`^[A-Z][A-Z ]+$`)

	i := 0
	for i < len(lines) {
		line := lines[i]

		// Check if this line starts an option definition
		if optionDefRe.MatchString(line) {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) == 0 || trimmed[0] != '-' {
				i++
				continue
			}

			// Skip lines that are too long to be option definitions
			// Description lines are typically much longer than option flag lines
			if len(trimmed) > maxOptionLineLength {
				i++
				continue
			}

			// Skip lines that look like comma-separated lists of multiple long options
			// These are typically found in body text, not actual definitions
			if multiOptionListRe.MatchString(trimmed) {
				i++
				continue
			}

			section := Section{
				StartLine: i,
			}

			optionIndent := len(line) - len(strings.TrimLeft(line, " \t"))

			// Extract the option text
			// Most options are just on a single line, so we start with just the trimmed line
			section.Option = trimmed
			i++

			// Skip collecting continuation lines - man pages typically have
			// options on one line and descriptions on subsequent indented lines
			// If there are multi-line options (rare), they would have already been
			// trimmed and joined when we got 'trimmed' above

			// Now collect the explanation (more indented lines)
			var explanationLines []string
			for i < len(lines) {
				nextLine := lines[i]

				// Empty line might separate paragraphs within the same option
				if strings.TrimSpace(nextLine) == "" {
					// Look ahead to see if next non-empty line is still explanation
					j := i + 1
					for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
						j++
					}
					if j < len(lines) {
						peekLine := lines[j]
						peekIndent := len(peekLine) - len(strings.TrimLeft(peekLine, " \t"))
						// If next content is still indented (explanation continues)
						if peekIndent > optionIndent+2 && !optionDefRe.MatchString(peekLine) {
							explanationLines = append(explanationLines, "")
							i++
							continue
						}
					}
					break
				}

				// Check if this is a new option definition or section header
				if optionDefRe.MatchString(nextLine) || sectionHeaderRe.MatchString(strings.TrimSpace(nextLine)) {
					break
				}

				// Check if this line is actually part of the explanation (more indented)
				nextIndent := len(nextLine) - len(strings.TrimLeft(nextLine, " \t"))
				if nextIndent <= optionIndent {
					// Not indented enough to be an explanation, stop here
					break
				}

				explanationLines = append(explanationLines, strings.TrimSpace(nextLine))
				i++
			}

			section.Explanation = strings.Join(explanationLines, " ")
			section.EndLine = i - 1

			// Only add if we have a valid option that starts with -
			if section.Option != "" && strings.HasPrefix(section.Option, "-") {
				sections = append(sections, section)
			}
		} else {
			i++
		}
	}

	return sections
}

// parseManSections extracts major section headers from man page lines
// These are lines that consist of all uppercase letters (e.g., NAME, SYNOPSIS, DESCRIPTION)
func parseManSections(lines []string) []ManSection {
	var sections []ManSection

	// Pattern: line starts with uppercase letter and contains only uppercase letters and spaces
	sectionHeaderRe := regexp.MustCompile(`^[A-Z][A-Z ]+$`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if sectionHeaderRe.MatchString(trimmed) {
			sections = append(sections, ManSection{
				Name:      trimmed,
				StartLine: i,
			})
		}
	}

	return sections
}

// MatchesQuery checks if a section matches the search query
// Searches both the option and explanation text (case-insensitive)
func (s Section) MatchesQuery(query string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(s.Option), query) ||
		strings.Contains(strings.ToLower(s.Explanation), query)
}

// extractOptionFlags extracts just the option flags from the Option field,
// excluding any description text that may follow.
// e.g., "-F      Display a slash..." -> "-F"
// e.g., "-r, --recursive   Copy recursively" -> "-r, --recursive"
func extractOptionFlags(option string) string {
	// Options are typically separated from description by multiple spaces
	// Find where the description starts (2+ consecutive spaces)
	for i := 0; i < len(option)-1; i++ {
		if option[i] == ' ' && option[i+1] == ' ' {
			return strings.TrimSpace(option[:i])
		}
	}
	// No double-space found, return the whole thing
	return option
}

// MatchesOption checks if a section's option flags match the search query (case-insensitive)
// Only searches within the actual option flags (e.g., "-F", "--force"), not description text
func (s Section) MatchesOption(query string) bool {
	if query == "" {
		return true
	}
	flags := extractOptionFlags(s.Option)
	return strings.Contains(strings.ToLower(flags), strings.ToLower(query))
}

// MatchesOptionExact checks if a section's option flags exactly match the search query (case-sensitive)
// Matches individual flags like "-F" or "--force" exactly, not partial matches
// Strips leading dashes for comparison, so "L" matches "-L" and "location" matches "--location"
func (s Section) MatchesOptionExact(query string) bool {
	if query == "" {
		return true
	}
	flags := extractOptionFlags(s.Option)
	queryNorm := strings.TrimLeft(query, "-")

	// Split flags by comma and whitespace to get individual options
	// e.g., "-r, --recursive" -> ["-r", "--recursive"]
	parts := strings.FieldsFunc(flags, func(r rune) bool {
		return r == ',' || r == ' '
	})

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Strip leading dashes from the flag for comparison (case-sensitive)
		partNorm := strings.TrimLeft(part, "-")
		if partNorm == queryNorm {
			return true
		}
	}
	return false
}

// MatchesDescription checks if a section's description matches the search query (case-insensitive)
func (s Section) MatchesDescription(query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s.Explanation), strings.ToLower(query))
}

// FilterSections returns sections that match the query
func FilterSections(sections []Section, query string) []Section {
	if query == "" {
		return sections
	}

	var filtered []Section
	for _, s := range sections {
		if s.MatchesQuery(query) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
