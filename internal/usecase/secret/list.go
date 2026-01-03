package secret

import (
	"context"
	"fmt"
	"regexp"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/parallel"
)

// ListClient is the interface for the list use case.
type ListClient interface {
	secretapi.ListSecretsAPI
	secretapi.GetSecretValueAPI
}

// ListInput holds input for the list use case.
type ListInput struct {
	Prefix    string // AWS-side name filter (substring match)
	Filter    string // Regex filter pattern (client-side)
	WithValue bool   // Include secret values
}

// ListEntry represents a single secret in list output.
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

	// Build list input with optional prefix filter
	listInput := &secretapi.ListSecretsInput{}
	if input.Prefix != "" {
		listInput.Filters = []secretapi.Filter{
			{
				Key:    secretapi.FilterNameStringTypeName,
				Values: []string{input.Prefix},
			},
		}
	}

	// Collect all secret names
	var names []string
	paginator := secretapi.NewListSecretsPaginator(u.Client, listInput)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, s := range page.SecretList {
			name := lo.FromPtr(s.Name)
			if filterRegex != nil && !filterRegex.MatchString(name) {
				continue
			}
			names = append(names, name)
		}
	}

	output := &ListOutput{}

	// If values are not requested, return names only
	if !input.WithValue {
		for _, name := range names {
			output.Entries = append(output.Entries, ListEntry{Name: name})
		}
		return output, nil
	}

	// Fetch values in parallel
	namesMap := make(map[string]struct{}, len(names))
	for _, name := range names {
		namesMap[name] = struct{}{}
	}

	results := parallel.ExecuteMap(ctx, namesMap, func(ctx context.Context, name string, _ struct{}) (string, error) {
		out, err := u.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
			SecretId: lo.ToPtr(name),
		})
		if err != nil {
			return "", err
		}
		return lo.FromPtr(out.SecretString), nil
	})

	// Collect results
	for _, name := range names {
		result := results[name]
		entry := ListEntry{Name: name}
		if result.Err != nil {
			entry.Error = result.Err
		} else {
			entry.Value = lo.ToPtr(result.Value)
		}
		output.Entries = append(output.Entries, entry)
	}

	return output, nil
}
