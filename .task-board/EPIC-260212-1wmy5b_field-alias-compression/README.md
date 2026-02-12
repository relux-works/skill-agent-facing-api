# EPIC-260212-1wmy5b: field-alias-compression

## Description
Research and potentially implement field name abbreviations (aliases) in Schema to further compress compact output. Core questions: (1) How much do short field names actually save in tokens? (2) Does the LLM pay a cognitive tax to maintain the alias→field mapping in context? (3) How often would the LLM round-trip to schema() to refresh the dictionary? Net savings may be negative if the mapping overhead exceeds the compression gain.

## Scope
(define epic scope)

## Acceptance Criteria
(define acceptance criteria)
