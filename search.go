package main

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ManPage represents a single man page entry from search results
type ManPage struct {
	Name        string
	Section     string
	Description string
}

// String returns a formatted display string for the man page
func (m ManPage) String() string {
	return m.Name + "(" + m.Section + ") - " + m.Description
}

// parseSectionPrefix checks if the keyword starts with a section number.
// If the keyword starts with a number followed by a space (e.g., "1 curl"),
// it returns the section and the remaining keyword.
// Otherwise, it returns empty section and the original keyword.
func parseSectionPrefix(keyword string) (section, searchTerm string) {
	keyword = strings.TrimSpace(keyword)
	if len(keyword) == 0 {
		return "", keyword
	}

	// Check if first character is a digit
	if !unicode.IsDigit(rune(keyword[0])) {
		return "", keyword
	}

	// Find the end of the section number (could be "1", "3p", "1ssl", etc.)
	spaceIdx := strings.Index(keyword, " ")
	if spaceIdx == -1 {
		// No space found, treat entire input as search term
		return "", keyword
	}

	section = keyword[:spaceIdx]
	searchTerm = strings.TrimSpace(keyword[spaceIdx+1:])

	// Validate section format: starts with digit, optionally followed by letters
	if len(section) == 0 || searchTerm == "" {
		return "", keyword
	}

	return section, searchTerm
}

// SearchManPages executes 'man -k <keyword>' and parses the results.
// If keyword starts with a section number (e.g., "1 curl"), searches only that section.
func SearchManPages(keyword string) ([]ManPage, error) {
	section, searchTerm := parseSectionPrefix(keyword)

	// Always search without -S flag, then filter by section in code.
	// macOS's man -S can miss exact matches like "ls" when searching "1 ls".
	var cmd *exec.Cmd
	if section != "" {
		cmd = exec.Command("man", "-k", searchTerm)
	} else {
		cmd = exec.Command("man", "-k", keyword)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// man -k returns exit code 1 when no results found
		if stderr.Len() > 0 && strings.Contains(stderr.String(), "nothing appropriate") {
			return []ManPage{}, nil
		}
		// Check if it's just "nothing appropriate" in stdout
		if strings.Contains(stdout.String(), "nothing appropriate") {
			return []ManPage{}, nil
		}
		// Some systems return exit 1 but still have valid output
		if stdout.Len() == 0 {
			return []ManPage{}, nil
		}
	}

	results := parseManOutput(stdout.String())

	// Filter by section if specified
	if section != "" {
		filtered := make([]ManPage, 0, len(results))
		for _, page := range results {
			if page.Section == section {
				filtered = append(filtered, page)
			}
		}
		results = filtered
	}

	sortManPages(results, searchTerm)
	return results, nil
}

// parseManOutput parses the output of 'man -k' into ManPage structs
// Format: name(section) - description
// Or: name, name2(section) - description (multiple names)
func parseManOutput(output string) []ManPage {
	var results []ManPage
	seen := make(map[string]bool)

	// Regex to match: name(section) - description
	// Also handles: name(sec), name2(sec) - description (multiple names with sections)
	// Uses .* to greedily match everything up to the last (section) before " - "
	re := regexp.MustCompile(`^(.*)\(([^)]+)\)\s+-\s+(.*)$`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if matches != nil {
			namesStr := strings.TrimSpace(matches[1])
			lastSection := strings.TrimSpace(matches[2])
			description := strings.TrimSpace(matches[3])

			// Parse all names from formats like:
			// "opendir(3), readdir(3), closedir(3)" or "grep, egrep, fgrep"
			// Each name might have its own (section) or share the last one
			nameRe := regexp.MustCompile(`([a-zA-Z0-9_.-]+)(?:\(([^)]+)\))?`)
			nameMatches := nameRe.FindAllStringSubmatch(namesStr, -1)

			for _, nm := range nameMatches {
				name := strings.TrimSpace(nm[1])
				section := lastSection
				if nm[2] != "" {
					section = strings.TrimSpace(nm[2])
				}

				// Deduplicate by name+section
				key := name + "(" + section + ")"
				if seen[key] {
					continue
				}
				seen[key] = true

				results = append(results, ManPage{
					Name:        name,
					Section:     section,
					Description: description,
				})
			}
		}
	}

	return results
}

// sortManPages sorts man pages so that:
// 1. Exact prefix matches (names starting with keyword) come first
// 2. Within each group, results are sorted alphabetically by name
func sortManPages(pages []ManPage, keyword string) {
	keywordLower := strings.ToLower(keyword)
	sort.Slice(pages, func(i, j int) bool {
		nameI := strings.ToLower(pages[i].Name)
		nameJ := strings.ToLower(pages[j].Name)

		prefixI := strings.HasPrefix(nameI, keywordLower)
		prefixJ := strings.HasPrefix(nameJ, keywordLower)

		// Prefix matches come first
		if prefixI && !prefixJ {
			return true
		}
		if !prefixI && prefixJ {
			return false
		}

		// Within the same group, sort alphabetically
		return nameI < nameJ
	})
}
