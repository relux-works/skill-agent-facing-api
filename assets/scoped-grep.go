// Reference implementation: scoped grep.
//
// Full-text regex search scoped to a data directory.
// Returns file:line:content matches with optional context lines.
//
// Adapt dataDir, file extensions, and output format to your domain.

package search

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Match represents a single grep hit.
type Match struct {
	Path    string `json:"path"`    // relative to data directory
	Line    int    `json:"line"`    // 1-indexed
	Content string `json:"content"` // matched line text
}

// Options controls grep behavior.
type Options struct {
	FileGlob       string // filter files by name glob (e.g. "progress.md", "*.md")
	CaseInsensitive bool
	ContextLines   int    // lines before and after each match
}

// Grep searches dataDir recursively for lines matching pattern.
// ADAPT: change file extension filter (.md) to match your data format.
func Grep(dataDir, pattern string, opts Options) ([]Match, error) {
	if opts.CaseInsensitive {
		pattern = "(?i)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	var results []Match

	err = filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// ADAPT THIS: filter to your data file extensions
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		// Apply file glob filter
		if opts.FileGlob != "" {
			matched, _ := filepath.Match(opts.FileGlob, d.Name())
			if !matched {
				return nil
			}
		}

		relPath, _ := filepath.Rel(dataDir, path)
		matches, _ := grepFile(path, relPath, re, opts.ContextLines)
		results = append(results, matches...)
		return nil
	})

	if results == nil {
		results = []Match{}
	}
	return results, err
}

func grepFile(path, relPath string, re *regexp.Regexp, contextLines int) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Find matching line numbers
	var matchNums []int
	for i, line := range lines {
		if re.MatchString(line) {
			matchNums = append(matchNums, i+1)
		}
	}
	if len(matchNums) == 0 {
		return nil, nil
	}

	// No context — return exact matches only
	if contextLines <= 0 {
		results := make([]Match, len(matchNums))
		for i, num := range matchNums {
			results[i] = Match{Path: relPath, Line: num, Content: lines[num-1]}
		}
		return results, nil
	}

	// With context — expand window around each match
	include := make(map[int]bool)
	for _, num := range matchNums {
		start := num - contextLines
		if start < 1 {
			start = 1
		}
		end := num + contextLines
		if end > len(lines) {
			end = len(lines)
		}
		for n := start; n <= end; n++ {
			include[n] = true
		}
	}

	var results []Match
	for n := 1; n <= len(lines); n++ {
		if include[n] {
			results = append(results, Match{Path: relPath, Line: n, Content: lines[n-1]})
		}
	}
	return results, nil
}

// PrintJSON outputs matches as JSON array.
func PrintJSON(matches []Match) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(matches)
}

// PrintText outputs matches in ripgrep-style format: path:line:content
func PrintText(matches []Match, contextLines int) {
	prevPath := ""
	prevLine := 0
	for _, m := range matches {
		if prevPath == m.Path && contextLines > 0 && m.Line > prevLine+1 {
			fmt.Println("--")
		}
		fmt.Printf("%s:%d:%s\n", m.Path, m.Line, m.Content)
		prevPath = m.Path
		prevLine = m.Line
	}
}
