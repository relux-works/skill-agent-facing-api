package agentquery

import (
	"fmt"
	"strings"
)

// Mutation registers a named mutation with its handler function.
// The mutation is stored in the mutations map and also wrapped into the
// operations map so the parser accepts the name and executeStatement can
// dispatch it.
func (s *Schema[T]) Mutation(name string, handler MutationHandler[T]) {
	s.mutations[name] = handler
	s.operations[name] = s.wrapMutation(name, handler)
}

// MutationWithMetadata registers a named mutation with its handler and metadata.
// Metadata is stored separately for schema introspection.
func (s *Schema[T]) MutationWithMetadata(name string, handler MutationHandler[T], meta MutationMetadata) {
	s.Mutation(name, handler)
	s.mutationMetadata[name] = meta
}

// HasMutations returns true if any mutations are registered on the schema.
func (s *Schema[T]) HasMutations() bool {
	return len(s.mutations) > 0
}

// IsMutationDestructive returns true if the named mutation has metadata with
// Destructive=true. Returns false for unknown mutations or mutations without metadata.
func (s *Schema[T]) IsMutationDestructive(name string) bool {
	if meta, ok := s.mutationMetadata[name]; ok {
		return meta.Destructive
	}
	return false
}

// wrapMutation adapts a MutationHandler into an OperationHandler.
// It builds ArgMap, extracts dry_run, runs framework-level validation
// from metadata (if registered), calls the handler, and wraps the result
// into a MutationResult.
func (s *Schema[T]) wrapMutation(name string, handler MutationHandler[T]) OperationHandler[T] {
	return func(ctx OperationContext[T]) (any, error) {
		// Build ArgMap from key=value args.
		argMap := make(map[string]string, len(ctx.Statement.Args))
		for _, arg := range ctx.Statement.Args {
			if arg.Key != "" {
				argMap[arg.Key] = arg.Value
			}
		}

		// Check dry_run flag and remove it from argMap.
		dryRun := false
		if v, ok := argMap["dry_run"]; ok && (v == "true" || v == "1" || v == "yes") {
			dryRun = true
			delete(argMap, "dry_run")
		}

		mctx := MutationContext[T]{
			Mutation:   name,
			Statement:  ctx.Statement,
			Args:       ctx.Statement.Args,
			ArgMap:     argMap,
			Items:      ctx.Items,
			DryRun:     dryRun,
		}

		// Framework-level validation from metadata (if registered).
		if meta, ok := s.mutationMetadata[name]; ok {
			if errs := validateMutationArgs(mctx.ArgMap, mctx.Args, meta.Parameters); len(errs) > 0 {
				return MutationResult{Ok: false, Errors: errs}, nil
			}
		}

		result, err := handler(mctx)
		if err != nil {
			return MutationResult{
				Ok:     false,
				Errors: []MutationError{{Message: err.Error()}},
			}, nil
		}

		return MutationResult{Ok: true, Result: result}, nil
	}
}

// validateMutationArgs checks required params and enum constraints from metadata.
// Returns nil if all checks pass. This is structural (Layer 1) validation;
// domain (Layer 2) validation belongs in the handler.
func validateMutationArgs(argMap map[string]string, args []Arg, params []ParameterDef) []MutationError {
	var errs []MutationError
	for _, p := range params {
		if p.Required {
			if _, ok := argMap[p.Name]; !ok {
				// Check positional args — a positional arg might satisfy
				// a required parameter if it's the first one (e.g. ID).
				found := false
				for _, a := range args {
					if a.Key == "" && a.Value != "" {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, MutationError{
						Field:   p.Name,
						Message: fmt.Sprintf("required parameter %q is missing", p.Name),
						Code:    ErrRequired,
					})
				}
			}
		}

		// Enum validation: if the param has an enum constraint and was provided.
		if len(p.Enum) > 0 {
			if val, ok := argMap[p.Name]; ok {
				valid := false
				for _, e := range p.Enum {
					if strings.EqualFold(val, e) {
						valid = true
						break
					}
				}
				if !valid {
					errs = append(errs, MutationError{
						Field:   p.Name,
						Message: fmt.Sprintf("invalid value %q for %s, must be one of: %s", val, p.Name, strings.Join(p.Enum, ", ")),
						Code:    ErrInvalidValue,
					})
				}
			}
		}
	}
	return errs
}
