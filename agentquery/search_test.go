package agentquery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestDir creates a temp directory with sample files for search tests.
// Structure:
//
//	dataDir/
//	  task.md          — "Status: done\nTitle: Fix login bug"
//	  notes.md         — "Some random notes\nNothing interesting"
//	  readme.txt       — "This is a readme\nStatus: active"
//	  sub/
//	    nested.md      — "Nested file\nStatus: blocked\nMore content"
//	    deep/
//	      deep.md      — "Deep nested\nStatus: done again"
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, dir, "task.md", "Status: done\nTitle: Fix login bug\nPriority: high")
	writeFile(t, dir, "notes.md", "Some random notes\nNothing interesting\nJust filler text")
	writeFile(t, dir, "readme.txt", "This is a readme\nStatus: active\nEnd of file")

	sub := filepath.Join(dir, "sub")
	must(t, os.MkdirAll(sub, 0o755))
	writeFile(t, sub, "nested.md", "Nested file\nStatus: blocked\nMore content\nExtra line")

	deep := filepath.Join(sub, "deep")
	must(t, os.MkdirAll(deep, 0o755))
	writeFile(t, deep, "deep.md", "Deep nested\nStatus: done again\nFinal line")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearch_SimpleMatch(t *testing.T) {
	dir := setupTestDir(t)

	results, err := Search(dir, "Fix login", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Source.Path != "task.md" {
		t.Errorf("expected path task.md, got %s", r.Source.Path)
	}
	if r.Source.Line != 2 {
		t.Errorf("expected line 2, got %d", r.Source.Line)
	}
	if r.Content != "Title: Fix login bug" {
		t.Errorf("expected 'Title: Fix login bug', got %q", r.Content)
	}
	if !r.IsMatch {
		t.Error("expected IsMatch=true for actual match")
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	dir := setupTestDir(t)

	// "fix login" should not match with case-sensitive (lowercase 'f').
	results, err := Search(dir, "fix login", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("case-sensitive: expected 0 results, got %d", len(results))
	}

	// With CaseInsensitive, it should match.
	results, err = Search(dir, "fix login", []string{".md"}, SearchOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("case-insensitive: expected 1 result, got %d", len(results))
	}
	if results[0].Content != "Title: Fix login bug" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}
}

func TestSearch_FileGlobFilter(t *testing.T) {
	dir := setupTestDir(t)

	// Search only "task.md" files for "Status".
	results, err := Search(dir, "Status", []string{".md"}, SearchOptions{FileGlob: "task.md"})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Source.Path != "task.md" {
		t.Errorf("expected task.md, got %s", results[0].Source.Path)
	}
}

func TestSearch_ExtensionFilter(t *testing.T) {
	dir := setupTestDir(t)

	// Search only .md files — readme.txt has "Status" but should be excluded.
	results, err := Search(dir, "Status", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results {
		ext := filepath.Ext(r.Source.Path)
		if ext != ".md" {
			t.Errorf("non-.md file in results: %s", r.Source.Path)
		}
	}

	// Now search all files (empty extensions).
	allResults, err := Search(dir, "Status", nil, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Should include readme.txt result.
	hasTxt := false
	for _, r := range allResults {
		if filepath.Ext(r.Source.Path) == ".txt" {
			hasTxt = true
		}
	}
	if !hasTxt {
		t.Error("expected .txt file in results when no extension filter")
	}

	if len(allResults) <= len(results) {
		t.Errorf("expected more results without extension filter: all=%d, md-only=%d", len(allResults), len(results))
	}
}

func TestSearch_ContextLines(t *testing.T) {
	dir := setupTestDir(t)

	// task.md has 3 lines: "Status: done", "Title: Fix login bug", "Priority: high"
	// Matching "Title" (line 2) with 1 context line should give lines 1-3.
	results, err := Search(dir, "Title", []string{".md"}, SearchOptions{ContextLines: 1})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results with context, got %d", len(results))
	}

	// Line 1: context (before match).
	if results[0].IsMatch {
		t.Error("line 1 should be context, got IsMatch=true")
	}
	if results[0].Source.Line != 1 {
		t.Errorf("expected line 1, got %d", results[0].Source.Line)
	}

	// Line 2: actual match.
	if !results[1].IsMatch {
		t.Error("line 2 should be match, got IsMatch=false")
	}
	if results[1].Source.Line != 2 {
		t.Errorf("expected line 2, got %d", results[1].Source.Line)
	}

	// Line 3: context (after match).
	if results[2].IsMatch {
		t.Error("line 3 should be context, got IsMatch=true")
	}
	if results[2].Source.Line != 3 {
		t.Errorf("expected line 3, got %d", results[2].Source.Line)
	}
}

func TestSearch_RegexPattern(t *testing.T) {
	dir := setupTestDir(t)

	// "Status.*done" should match lines with "Status: done" and "Status: done again".
	results, err := Search(dir, "Status.*done", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 regex results, got %d", len(results))
	}

	for _, r := range results {
		if !r.IsMatch {
			t.Errorf("expected IsMatch=true for regex match: %q", r.Content)
		}
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	dir := setupTestDir(t)

	results, err := Search(dir, "nonexistent_pattern_xyz", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if results == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_InvalidRegex(t *testing.T) {
	dir := setupTestDir(t)

	_, err := Search(dir, "[invalid(regex", []string{".md"}, SearchOptions{})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if e.Code != ErrParse {
		t.Errorf("expected ErrParse code, got %s", e.Code)
	}
}

func TestSearch_RelativePaths(t *testing.T) {
	dir := setupTestDir(t)

	results, err := Search(dir, "Status", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results {
		if filepath.IsAbs(r.Source.Path) {
			t.Errorf("path should be relative, got absolute: %s", r.Source.Path)
		}
	}
}

func TestSearch_NestedDirectories(t *testing.T) {
	dir := setupTestDir(t)

	results, err := Search(dir, "Status", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Should find matches in task.md, sub/nested.md, sub/deep/deep.md.
	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Source.Path] = true
	}

	expected := []string{"task.md", filepath.Join("sub", "nested.md"), filepath.Join("sub", "deep", "deep.md")}
	for _, p := range expected {
		if !paths[p] {
			t.Errorf("expected match in %s, not found in results", p)
		}
	}
}

func TestSearch_AllExtensions(t *testing.T) {
	dir := setupTestDir(t)

	// Empty extensions = search all files.
	results, err := Search(dir, "readme", nil, SearchOptions{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results from .txt file when searching all extensions")
	}

	hasTxt := false
	for _, r := range results {
		if filepath.Ext(r.Source.Path) == ".txt" {
			hasTxt = true
		}
	}
	if !hasTxt {
		t.Error("expected .txt file in results")
	}
}

func TestSearch_ExtensionWithoutDot(t *testing.T) {
	dir := setupTestDir(t)

	// Extensions without leading dot should still work.
	results, err := Search(dir, "Status", []string{"md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results when extension specified without dot")
	}

	for _, r := range results {
		ext := filepath.Ext(r.Source.Path)
		if ext != ".md" {
			t.Errorf("non-.md file in results: %s", r.Source.Path)
		}
	}
}

func TestSearch_FileGlobWildcard(t *testing.T) {
	dir := setupTestDir(t)

	// Glob "*.md" should match all .md files but the extension filter also applies.
	results, err := Search(dir, "Status", []string{".md"}, SearchOptions{FileGlob: "*.md"})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results with wildcard glob")
	}
}

func TestSearchJSON(t *testing.T) {
	dir := setupTestDir(t)

	data, err := SearchJSON(dir, "Fix login", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var results []SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result from JSON, got %d", len(results))
	}
	if results[0].Source.Path != "task.md" {
		t.Errorf("expected task.md, got %s", results[0].Source.Path)
	}
}

func TestSearchJSON_EmptyResults(t *testing.T) {
	dir := setupTestDir(t)

	data, err := SearchJSON(dir, "nonexistent_xyz", []string{".md"}, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var results []SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_ContextLinesOverlap(t *testing.T) {
	dir := setupTestDir(t)

	// sub/nested.md: "Nested file\nStatus: blocked\nMore content\nExtra line"
	// Match "Nested" (line 1) and "More content" (line 3) with 1 context line.
	// Windows overlap: lines 1-2 and 2-4 → all 4 lines, no duplicates.
	results, err := Search(dir, "Nested|More content", []string{".md"}, SearchOptions{
		ContextLines: 1,
		FileGlob:     "nested.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results (overlapping context), got %d", len(results))
	}

	// Line 1: match ("Nested file").
	if !results[0].IsMatch {
		t.Error("line 1 should be a match")
	}
	// Line 2: context for both matches.
	if results[1].IsMatch {
		t.Error("line 2 should be context only")
	}
	// Line 3: match ("More content").
	if !results[2].IsMatch {
		t.Error("line 3 should be a match")
	}
	// Line 4: context.
	if results[3].IsMatch {
		t.Error("line 4 should be context only")
	}
}

func TestSearchJSON_InvalidRegex(t *testing.T) {
	dir := setupTestDir(t)

	_, err := SearchJSON(dir, "[bad", []string{".md"}, SearchOptions{})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrParse {
		t.Errorf("expected ErrParse, got %s", e.Code)
	}
}

// --- Tests for extracted public functions ---

func TestCompilePattern_Valid(t *testing.T) {
	re, err := CompilePattern("Status.*done", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !re.MatchString("Status: done") {
		t.Error("expected match")
	}
	if re.MatchString("status: done") {
		t.Error("should be case-sensitive")
	}
}

func TestCompilePattern_CaseInsensitive(t *testing.T) {
	re, err := CompilePattern("status", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !re.MatchString("Status: done") {
		t.Error("expected case-insensitive match")
	}
}

func TestCompilePattern_InvalidRegex(t *testing.T) {
	_, err := CompilePattern("[bad", false)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Code != ErrParse {
		t.Errorf("expected ErrParse, got %s", e.Code)
	}
}

func TestMatchLines_NoMatch(t *testing.T) {
	lines := []string{"hello", "world"}
	re, _ := CompilePattern("xyz", false)
	results := MatchLines(lines, "test.md", re, 0)
	if results != nil {
		t.Errorf("expected nil for no matches, got %d results", len(results))
	}
}

func TestMatchLines_ExactMatch(t *testing.T) {
	lines := []string{"Status: done", "Title: Fix login bug", "Priority: high"}
	re, _ := CompilePattern("Title", false)
	results := MatchLines(lines, "test.md", re, 0)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Source.Path != "test.md" {
		t.Errorf("path = %s, want test.md", results[0].Source.Path)
	}
	if results[0].Source.Line != 2 {
		t.Errorf("line = %d, want 2", results[0].Source.Line)
	}
	if !results[0].IsMatch {
		t.Error("expected IsMatch=true")
	}
}

func TestMatchLines_WithContext(t *testing.T) {
	lines := []string{"line1", "line2", "match", "line4", "line5"}
	re, _ := CompilePattern("match", false)
	results := MatchLines(lines, "f.md", re, 1)

	if len(results) != 3 {
		t.Fatalf("expected 3 results (match + 2 context), got %d", len(results))
	}
	if results[0].Source.Line != 2 || results[0].IsMatch {
		t.Error("line 2 should be context")
	}
	if results[1].Source.Line != 3 || !results[1].IsMatch {
		t.Error("line 3 should be match")
	}
	if results[2].Source.Line != 4 || results[2].IsMatch {
		t.Error("line 4 should be context")
	}
}

func TestMatchLines_OverlappingContext(t *testing.T) {
	lines := []string{"a", "match1", "b", "match2", "c"}
	re, _ := CompilePattern("match", false)
	results := MatchLines(lines, "f.md", re, 1)

	// Windows: [1-3] and [3-5] → all 5 lines, no dupes
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	if !results[1].IsMatch || !results[3].IsMatch {
		t.Error("match lines should have IsMatch=true")
	}
}

// --- FormatSearchCompact tests ---

func TestFormatSearchCompact_Empty(t *testing.T) {
	out := FormatSearchCompact(nil)
	if len(out) != 0 {
		t.Errorf("expected empty output for nil results, got %q", string(out))
	}

	out = FormatSearchCompact([]SearchResult{})
	if len(out) != 0 {
		t.Errorf("expected empty output for empty results, got %q", string(out))
	}
}

func TestFormatSearchCompact_SingleFile(t *testing.T) {
	results := []SearchResult{
		{Source: Source{Path: "task.md", Line: 1}, Content: "Status: done", IsMatch: true},
		{Source: Source{Path: "task.md", Line: 2}, Content: "Title: Fix login bug", IsMatch: true},
	}

	out := string(FormatSearchCompact(results))
	expected := "task.md\n  1: Status: done\n  2: Title: Fix login bug\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}
}

func TestFormatSearchCompact_MultipleFiles(t *testing.T) {
	results := []SearchResult{
		{Source: Source{Path: "README.md", Line: 3}, Content: "matching line", IsMatch: true},
		{Source: Source{Path: "README.md", Line: 4}, Content: "context line", IsMatch: false},
		{Source: Source{Path: "other.md", Line: 12}, Content: "another match", IsMatch: true},
	}

	out := string(FormatSearchCompact(results))
	expected := "README.md\n  3: matching line\n  4  context line\n\nother.md\n  12: another match\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}
}

func TestFormatSearchCompact_MatchVsContext(t *testing.T) {
	results := []SearchResult{
		{Source: Source{Path: "f.md", Line: 1}, Content: "before", IsMatch: false},
		{Source: Source{Path: "f.md", Line: 2}, Content: "match", IsMatch: true},
		{Source: Source{Path: "f.md", Line: 3}, Content: "after", IsMatch: false},
	}

	out := string(FormatSearchCompact(results))

	// Match line uses ":"
	if !strings.Contains(out, "  2: match\n") {
		t.Errorf("expected match line with colon separator, got:\n%s", out)
	}

	// Context lines use space (no colon)
	if !strings.Contains(out, "  1  before\n") {
		t.Errorf("expected context line with space separator, got:\n%s", out)
	}
	if !strings.Contains(out, "  3  after\n") {
		t.Errorf("expected context line with space separator, got:\n%s", out)
	}
}

func TestFormatSearchCompact_GroupOrder(t *testing.T) {
	// Results interleaved from different files — should be grouped by first appearance.
	results := []SearchResult{
		{Source: Source{Path: "b.md", Line: 1}, Content: "first b", IsMatch: true},
		{Source: Source{Path: "a.md", Line: 5}, Content: "first a", IsMatch: true},
		{Source: Source{Path: "b.md", Line: 3}, Content: "second b", IsMatch: true},
	}

	out := string(FormatSearchCompact(results))

	// b.md should come before a.md (first appearance order)
	bIdx := strings.Index(out, "b.md")
	aIdx := strings.Index(out, "a.md")
	if bIdx >= aIdx {
		t.Errorf("expected b.md before a.md in output:\n%s", out)
	}

	// b.md group should have both results
	expected := "b.md\n  1: first b\n  3: second b\n\na.md\n  5: first a\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}
}

// --- Schema.SearchJSON with LLMReadable mode ---

func TestSchemaSearchJSON_LLMReadable(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSchema[struct{}](
		WithDataDir(dir),
		WithExtensions(".md"),
		WithOutputMode(LLMReadable),
	)

	data, err := s.SearchJSON("Fix login", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)

	// Should NOT be JSON
	if json.Valid(data) && strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Error("LLMReadable output should not be JSON array")
	}

	// Should contain file path header and match line
	if !strings.Contains(out, "task.md") {
		t.Errorf("expected file path header, got:\n%s", out)
	}
	if !strings.Contains(out, "2: Title: Fix login bug") {
		t.Errorf("expected match line, got:\n%s", out)
	}
}

func TestSchemaSearchJSON_HumanReadable(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSchema[struct{}](
		WithDataDir(dir),
		WithExtensions(".md"),
		// Default HumanReadable — no WithOutputMode
	)

	data, err := s.SearchJSON("Fix login", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("HumanReadable output should be valid JSON: %s", string(data))
	}

	var results []SearchResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSchemaSearchJSONWithMode_OverridesDefault(t *testing.T) {
	dir := setupTestDir(t)

	// Schema defaults to HumanReadable, but we override with LLMReadable
	s := NewSchema[struct{}](
		WithDataDir(dir),
		WithExtensions(".md"),
	)

	data, err := s.SearchJSONWithMode("Fix login", SearchOptions{}, LLMReadable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "task.md") {
		t.Errorf("expected compact format with file header, got:\n%s", out)
	}
	if !strings.Contains(out, "2: Title: Fix login bug") {
		t.Errorf("expected compact match line, got:\n%s", out)
	}
}

func TestSchemaSearchJSON_LLMReadable_Empty(t *testing.T) {
	dir := setupTestDir(t)

	s := NewSchema[struct{}](
		WithDataDir(dir),
		WithExtensions(".md"),
		WithOutputMode(LLMReadable),
	)

	data, err := s.SearchJSON("nonexistent_xyz_pattern", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("expected empty output for no results in LLMReadable mode, got %q", string(data))
	}
}

func TestFileSystemSearchProvider(t *testing.T) {
	dir := setupTestDir(t)
	p := &FileSystemSearchProvider{DataDir: dir, Extensions: []string{".md"}}

	results, err := p.Search("Fix login", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Source.Path != "task.md" {
		t.Errorf("path = %s, want task.md", results[0].Source.Path)
	}
}
