package azure

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
)

// ListInput holds input for the list use case.
type ListInput struct {
	Prefix    string // Name prefix filter (case-sensitive)
	Filter    string // Regex filter pattern (client-side)
	WithValue bool   // Include values
}

// ListEntry represents a single entry in list output.
type ListEntry struct {
	Name  string
	Value *string // nil when error or not requested
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

// Execute runs the list use case. The provider returns every name; the name
// prefix filter and the client-side regex filter are applied here.
func (u *ListUseCase) Execute(ctx context.Context, input ListInput) (*ListOutput, error) {
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
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	filtered := lo.Filter(names, func(name string, _ int) bool {
		if input.Prefix != "" && !strings.HasPrefix(name, input.Prefix) {
			return false
		}

		if filterRegex != nil && !filterRegex.MatchString(name) {
			return false
		}

		return true
	})

	// Distinguishes "the API returned nothing" from "the client-side filters
	// dropped everything" — the two look identical in the final output.
	debug.From(ctx).Logf("list: provider returned %d names, %d after filters (prefix=%q, filter=%q)\n",
		len(names), len(filtered), input.Prefix, input.Filter)

	return u.buildOutput(ctx, input.WithValue, filtered), nil
}

// buildOutput creates the output, fetching values in parallel when requested.
func (u *ListUseCase) buildOutput(ctx context.Context, withValue bool, names []string) *ListOutput {
	output := &ListOutput{}

	if !withValue {
		for _, name := range names {
			output.Entries = append(output.Entries, ListEntry{Name: name})
		}

		return output
	}

	values, errs := u.fetchValues(ctx, names)

	for _, name := range names {
		entry := ListEntry{Name: name}

		if err, hasErr := errs[name]; hasErr {
			entry.Error = err
		} else if val, hasVal := values[name]; hasVal {
			entry.Value = lo.ToPtr(val)
		}

		output.Entries = append(output.Entries, entry)
	}

	return output
}

// fetchValues retrieves each entry's current value in parallel.
func (u *ListUseCase) fetchValues(ctx context.Context, names []string) (map[string]string, map[string]error) {
	if len(names) == 0 {
		return nil, nil
	}

	nameMap := lo.SliceToMap(names, func(name string) (string, string) { return name, name })

	results := parallel.ExecuteMap(ctx, nameMap, func(ctx context.Context, _ string, name string) (string, error) {
		entry, err := u.Reader.Get(ctx, name, provider.VersionRef{})
		if err != nil {
			return "", err
		}

		return entry.Value, nil
	})

	values := make(map[string]string)
	errs := make(map[string]error)

	for name, result := range results {
		if result.Err != nil {
			errs[name] = result.Err

			continue
		}

		values[name] = result.Value
	}

	return values, errs
}
