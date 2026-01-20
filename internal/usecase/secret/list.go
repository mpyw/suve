package secret

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
)

// ListClient is the interface for the list use case.
//
//nolint:iface // Intentional type alias for semantic clarity
type ListClient interface {
	provider.SecretReader
}

// ListInput holds input for the list use case.
type ListInput struct {
	Prefix     string // Prefix filter (client-side substring match)
	Filter     string // Regex filter pattern (client-side)
	WithValue  bool   // Include secret values
	MaxResults int    // Max results per page (0 = all)
	NextToken  string // Pagination token
}

// ListEntry represents a single secret in list output.
type ListEntry struct {
	Name  string
	Value *string // nil when error or not requested
	Error error
}

// ListOutput holds the result of the list use case.
type ListOutput struct {
	Entries   []ListEntry
	NextToken string // Empty if no more pages
}

// ListUseCase executes list operations.
type ListUseCase struct {
	Client ListClient
}

// Execute runs the list use case.
func (u *ListUseCase) Execute(ctx context.Context, input ListInput) (*ListOutput, error) {
	// Compile regex filter if specified
	var filterRegex *regexp.Regexp

	if input.Filter != "" {
		var err error

		filterRegex, err = regexp.Compile(input.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	// Fetch all secrets via provider interface
	items, err := u.Client.ListSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	// Apply filters
	//nolint:prealloc // Can't pre-allocate when filtering
	var filtered []*model.SecretListItem

	for _, item := range items {
		// Apply prefix filter (substring match)
		if input.Prefix != "" && !strings.Contains(item.Name, input.Prefix) {
			continue
		}

		// Apply regex filter
		if filterRegex != nil && !filterRegex.MatchString(item.Name) {
			continue
		}

		filtered = append(filtered, item)
	}

	// Apply pagination (client-side since provider returns all)
	startIndex := 0

	if input.NextToken != "" {
		// Find the index after the NextToken
		for i, item := range filtered {
			if item.Name == input.NextToken {
				startIndex = i + 1

				break
			}
		}
	}

	// Slice from start index
	if startIndex > 0 && startIndex < len(filtered) {
		filtered = filtered[startIndex:]
	} else if startIndex >= len(filtered) {
		filtered = nil
	}

	// Apply MaxResults limit
	var nextToken string
	if input.MaxResults > 0 && len(filtered) > input.MaxResults {
		nextToken = filtered[input.MaxResults-1].Name
		filtered = filtered[:input.MaxResults]
	}

	// Build output
	return u.buildOutput(ctx, input.WithValue, filtered, nextToken)
}

// buildOutput creates the output with optional values.
func (u *ListUseCase) buildOutput(
	ctx context.Context, withValue bool, items []*model.SecretListItem, nextToken string,
) (*ListOutput, error) {
	output := &ListOutput{NextToken: nextToken}

	// If values are not requested, return names only
	if !withValue {
		for _, item := range items {
			output.Entries = append(output.Entries, ListEntry{Name: item.Name})
		}

		return output, nil
	}

	// Fetch values using parallel GetSecret calls
	values, errs := u.fetchValues(ctx, items)

	// Collect results in order
	for _, item := range items {
		entry := ListEntry{Name: item.Name}

		if err, hasErr := errs[item.Name]; hasErr {
			entry.Error = err
		} else if val, hasVal := values[item.Name]; hasVal {
			entry.Value = lo.ToPtr(val)
		}

		output.Entries = append(output.Entries, entry)
	}

	return output, nil
}

// fetchValues fetches secret values in parallel using GetSecret.
// Returns maps of name->value and name->error.
func (u *ListUseCase) fetchValues(
	ctx context.Context, items []*model.SecretListItem,
) (map[string]string, map[string]error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Create name map for parallel execution
	nameMap := make(map[string]string, len(items))
	for _, item := range items {
		nameMap[item.Name] = item.Name
	}

	// Execute in parallel
	results := parallel.ExecuteMap(ctx, nameMap, func(ctx context.Context, _ string, name string) (*model.Secret, error) {
		return u.Client.GetSecret(ctx, name, "", "")
	})

	// Collect results
	values := make(map[string]string)
	errs := make(map[string]error)

	for name, result := range results {
		if result.Err != nil {
			errs[name] = fmt.Errorf("failed to get secret: %w", result.Err)

			continue
		}

		values[name] = result.Value.Value
	}

	return values, errs
}
