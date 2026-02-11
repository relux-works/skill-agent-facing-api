# STORY-260211-e06u45: unified-search-facade

## Description
Integrate full-text regex search into the same library facade alongside DSL queries. Single entry point for both structured queries and unstructured text search. The grep method accepts a regex pattern, walks the user's data directory, and returns Source (file path + line number) + Result (matched content). User registers the data directory and file extensions once, then calls facade.Search(pattern, opts) alongside facade.Query(dslString). One import, one facade, all reads covered.

## Scope
(define story scope)

## Acceptance Criteria
- facade.Search(pattern string, opts SearchOpts) returns []SearchResult
- SearchResult has Source (FilePath + Line int) and Content (matched text)
- SearchOpts: FileGlob, CaseInsensitive, ContextLines
- Uses same data directory as DSL (registered once on facade init)
- File extension filter configured once at setup (e.g. .md, .yaml)
- Returns JSON-serializable results consistent with DSL output style
- Works alongside facade.Query() — one facade, two methods, all reads
- Regex compiled once, errors returned as structured JSON
