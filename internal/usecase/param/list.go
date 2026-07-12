package param

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
)

// ListInput holds input for the list use case.
type ListInput struct {
	Prefix    string
	Recursive bool
	Filter    string // Regex filter pattern
	WithValue bool   // Include parameter values
}

// ListEntry represents a single parameter in list output.
type ListEntry struct {
	Name  string
	Value *string // nil when error or not requested
	// Type is the provider-neutral value type. It is only known when values are
	// fetched (WithValue); for name-only listings it stays the zero value.
	Type  domain.ValueType
	Error error
}

// ListOutput holds the result of the list use case.
type ListOutput struct {
	Entries []ListEntry
}

// ListUseCase executes list operations.
type ListUseCase struct {
	Reader provider.Reader
}

// Execute runs the list use case.
func (u *ListUseCase) Execute(ctx context.Context, input ListInput) (*ListOutput, error) {
	// Compile regex filter if specified.
	var filterRegex *regexp.Regexp

	if input.Filter != "" {
		var err error

		filterRegex, err = regexp.Compile(input.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	names, err := u.Reader.List(ctx)
	if err != nil {
		return nil, err
	}

	// Apply prefix/recursive and regex filtering client-side.
	filtered := lo.Filter(names, func(name string, _ int) bool {
		if !MatchPrefix(name, input.Prefix, input.Recursive) {
			return false
		}

		if filterRegex != nil && !filterRegex.MatchString(name) {
			return false
		}

		return true
	})

	// Distinguishes "the API returned nothing" from "the client-side filters
	// dropped everything" — the two look identical in the final output.
	debug.From(ctx).Logf("aws ssm list: provider returned %d names, %d after filters (prefix=%q, recursive=%v, filter=%q)\n",
		len(names), len(filtered), input.Prefix, input.Recursive, input.Filter)

	// Sort names alphabetically so the listing has a stable, deterministic order
	// regardless of the provider API's native ordering (#480).
	slices.Sort(filtered)

	return u.buildOutput(ctx, input.WithValue, filtered), nil
}

// MatchPrefix reports whether name is in scope for the given prefix, using AWS
// Parameter Store PATH-HIERARCHY semantics (the old server-side Path filter):
//
//   - An empty prefix matches everything (the old code applied no Path filter,
//     returning all parameters at any depth).
//   - Otherwise a name matches the prefix iff name == prefix OR it is a
//     descendant, i.e. HasPrefix(name, prefix+"/"). This is hierarchical, so
//     prefix "/app" does NOT match "/application".
//   - Recursive (--recursive) matches any depth under the prefix.
//   - OneLevel (default) matches only immediate children: the segment after
//     "prefix/" must contain no further "/".
//
// A trailing slash on the prefix is normalized so "/app/" behaves like "/app".
func MatchPrefix(name, prefix string, recursive bool) bool {
	if prefix == "" {
		return true
	}

	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" { // prefix was only slashes → treat as root (match everything)
		return true
	}

	if name == prefix {
		return true
	}

	if !strings.HasPrefix(name, prefix+"/") {
		return false
	}

	if recursive {
		return true
	}

	rest := name[len(prefix)+1:] // segment(s) after "prefix/"

	return !strings.Contains(rest, "/")
}

// buildOutput creates the output, fetching values in parallel when requested.
func (u *ListUseCase) buildOutput(ctx context.Context, withValue bool, names []string) *ListOutput {
	output := &ListOutput{}

	if !withValue {
		output.Entries = lo.Map(names, func(name string, _ int) ListEntry {
			return ListEntry{Name: name}
		})

		return output
	}

	entries, errs := u.fetchEntries(ctx, names)

	output.Entries = lo.Map(names, func(name string, _ int) ListEntry {
		entry := ListEntry{Name: name}

		if err, hasErr := errs[name]; hasErr {
			entry.Error = err
		} else if e, hasEntry := entries[name]; hasEntry {
			entry.Value = lo.ToPtr(e.Value)
			entry.Type = e.Type
		}

		return entry
	})

	return output
}

// fetchEntries retrieves each parameter's latest entry in parallel, returning
// maps of name->entry and name->error.
func (u *ListUseCase) fetchEntries(ctx context.Context, names []string) (map[string]domain.Entry, map[string]error) {
	if len(names) == 0 {
		return nil, nil
	}

	nameMap := lo.SliceToMap(names, func(name string) (string, string) { return name, name })

	results := parallel.ExecuteMap(ctx, nameMap, func(ctx context.Context, _ string, name string) (domain.Entry, error) {
		entry, err := u.Reader.Get(ctx, name, provider.VersionRef{})
		if err != nil {
			return domain.Entry{}, err
		}

		return *entry, nil
	})

	entries := make(map[string]domain.Entry)
	errs := make(map[string]error)

	for name, result := range results {
		if result.Err != nil {
			errs[name] = result.Err

			continue
		}

		entries[name] = result.Value
	}

	return entries, errs
}
