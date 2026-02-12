package agentquery

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CompilePattern compiles a regex pattern, optionally with case-insensitive matching.
// Returns *Error{Code: ErrParse} for invalid regex patterns.
func CompilePattern(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &Error{
			Code:    ErrParse,
			Message: fmt.Sprintf("invalid regex: %s", err),
			Details: map[string]any{"pattern": pattern},
		}
	}
	return re, nil
}

// MatchLines scans lines for regex matches and returns SearchResult entries.
// sourcePath is used in Source.Path of each result (typically a relative path).
// When contextLines > 0, surrounding lines are included with IsMatch=false.
func MatchLines(lines []string, sourcePath string, re *regexp.Regexp, contextLines int) []SearchResult {
	// Find 1-indexed matching line numbers.
	matchSet := make(map[int]bool)
	var matchNums []int
	for i, line := range lines {
		if re.MatchString(line) {
			num := i + 1
			matchSet[num] = true
			matchNums = append(matchNums, num)
		}
	}

	if len(matchNums) == 0 {
		return nil
	}

	// No context — return exact matches only.
	if contextLines <= 0 {
		results := make([]SearchResult, len(matchNums))
		for i, num := range matchNums {
			results[i] = SearchResult{
				Source:  Source{Path: sourcePath, Line: num},
				Content: lines[num-1],
				IsMatch: true,
			}
		}
		return results
	}

	// With context — expand window around each match, mark context vs match.
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

	var results []SearchResult
	for n := 1; n <= len(lines); n++ {
		if include[n] {
			results = append(results, SearchResult{
				Source:  Source{Path: sourcePath, Line: n},
				Content: lines[n-1],
				IsMatch: matchSet[n],
			})
		}
	}
	return results
}

// Search performs a recursive full-text regex search within dataDir.
// It walks the directory tree, filters by extensions (e.g. []string{".md"}),
// and returns matching lines along with optional context lines.
//
// If extensions is empty, all files are searched.
// If opts.FileGlob is set, only files matching the glob pattern are searched.
// If opts.CaseInsensitive is true, the pattern is compiled with (?i).
// If opts.ContextLines > 0, surrounding lines are included with IsMatch=false.
//
// Returns *Error{Code: ErrParse} for invalid regex patterns.
// Returns an empty (non-nil) slice when no matches are found.
func Search(dataDir string, pattern string, extensions []string, opts SearchOptions) ([]SearchResult, error) {
	re, err := CompilePattern(pattern, opts.CaseInsensitive)
	if err != nil {
		return nil, err
	}

	// Build extension set for O(1) lookup.
	extSet := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		// Normalize: accept both ".md" and "md".
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[ext] = true
	}

	var results []SearchResult

	err = filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// Filter by extensions if any are specified.
		if len(extSet) > 0 {
			ext := filepath.Ext(d.Name())
			if !extSet[ext] {
				return nil
			}
		}

		// Apply file glob filter on filename.
		if opts.FileGlob != "" {
			matched, _ := filepath.Match(opts.FileGlob, d.Name())
			if !matched {
				return nil
			}
		}

		relPath, _ := filepath.Rel(dataDir, path)
		fileResults := searchFile(path, relPath, re, opts.ContextLines)
		results = append(results, fileResults...)
		return nil
	})

	if results == nil {
		results = []SearchResult{}
	}
	return results, err
}

// SearchJSON performs a search and returns the results as indented JSON bytes.
func SearchJSON(dataDir string, pattern string, extensions []string, opts SearchOptions) ([]byte, error) {
	results, err := Search(dataDir, pattern, extensions, opts)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(results, "", "  ")
}

// FileSystemSearchProvider implements SearchProvider by walking a directory tree.
type FileSystemSearchProvider struct {
	DataDir    string
	Extensions []string
}

// Search walks the filesystem and returns matching lines.
func (p *FileSystemSearchProvider) Search(pattern string, opts SearchOptions) ([]SearchResult, error) {
	return Search(p.DataDir, pattern, p.Extensions, opts)
}

// FormatSearchCompact formats search results in a compact grouped-by-file format.
// Match lines use ":" separator, context lines use " " (space).
// File path appears as a header line with no indentation, followed by indented lines.
// Blank line separates file groups. Returns empty []byte for empty results.
func FormatSearchCompact(results []SearchResult) []byte {
	if len(results) == 0 {
		return []byte{}
	}

	// Group results by file path, preserving order of first appearance.
	type fileGroup struct {
		path    string
		results []SearchResult
	}
	var groups []fileGroup
	groupIdx := make(map[string]int)

	for _, r := range results {
		idx, exists := groupIdx[r.Source.Path]
		if !exists {
			idx = len(groups)
			groupIdx[r.Source.Path] = idx
			groups = append(groups, fileGroup{path: r.Source.Path})
		}
		groups[idx].results = append(groups[idx].results, r)
	}

	var buf bytes.Buffer
	for i, g := range groups {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(g.path)
		buf.WriteByte('\n')
		for _, r := range g.results {
			if r.IsMatch {
				fmt.Fprintf(&buf, "  %d: %s\n", r.Source.Line, r.Content)
			} else {
				fmt.Fprintf(&buf, "  %d  %s\n", r.Source.Line, r.Content)
			}
		}
	}

	return buf.Bytes()
}

// searchFile scans a single file for regex matches and returns SearchResult entries.
// When contextLines > 0, surrounding lines are included with IsMatch=false.
func searchFile(path, relPath string, re *regexp.Regexp, contextLines int) []SearchResult {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return MatchLines(lines, relPath, re, contextLines)
}
