package param

import (
	"context"
	"fmt"
	"regexp"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/parallel"
)

// ListClient is the interface for the list use case.
type ListClient interface {
	paramapi.DescribeParametersAPI
	paramapi.GetParameterAPI
}

// ListInput holds input for the list use case.
type ListInput struct {
	Prefix     string
	Recursive  bool
	Filter     string // Regex filter pattern
	WithValue  bool   // Include parameter values
	MaxResults int    // Max results per page (0 = all)
	NextToken  string // Pagination token
}

// ListEntry represents a single parameter in list output.
type ListEntry struct {
	Name  string
	Type  string  // String, SecureString, or StringList
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

	option := "OneLevel"
	if input.Recursive {
		option = "Recursive"
	}

	// Build API input
	apiInput := &paramapi.DescribeParametersInput{}
	if input.Prefix != "" {
		apiInput.ParameterFilters = []paramapi.ParameterStringFilter{
			{
				Key:    lo.ToPtr("Path"),
				Option: lo.ToPtr(option),
				Values: []string{input.Prefix},
			},
		}
	}

	// Pagination mode: fetch one page at a time
	if input.MaxResults > 0 {
		return u.executeWithPagination(ctx, input, apiInput, filterRegex)
	}

	// Non-pagination mode: fetch all pages
	return u.executeAll(ctx, input, apiInput, filterRegex)
}

// paramInfo holds parameter name and type for internal processing.
type paramInfo struct {
	Name string
	Type string
}

// executeAll fetches all pages (original behavior).
func (u *ListUseCase) executeAll(ctx context.Context, input ListInput, apiInput *paramapi.DescribeParametersInput, filterRegex *regexp.Regexp) (*ListOutput, error) {
	var params []paramInfo
	paginator := paramapi.NewDescribeParametersPaginator(u.Client, apiInput)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, param := range page.Parameters {
			name := lo.FromPtr(param.Name)
			if filterRegex != nil && !filterRegex.MatchString(name) {
				continue
			}
			params = append(params, paramInfo{
				Name: name,
				Type: string(param.Type),
			})
		}
	}

	return u.buildOutput(ctx, input.WithValue, params, "")
}

// executeWithPagination fetches pages until MaxResults is reached or no more pages.
func (u *ListUseCase) executeWithPagination(ctx context.Context, input ListInput, apiInput *paramapi.DescribeParametersInput, filterRegex *regexp.Regexp) (*ListOutput, error) {
	var params []paramInfo
	nextToken := input.NextToken

	// AWS DescribeParameters max is 50, request more to account for filtering
	const awsMaxResults int32 = 50

	for {
		// Set pagination params
		apiInput.MaxResults = lo.ToPtr(awsMaxResults)
		if nextToken != "" {
			apiInput.NextToken = lo.ToPtr(nextToken)
		} else {
			apiInput.NextToken = nil
		}

		page, err := u.Client.DescribeParameters(ctx, apiInput)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, param := range page.Parameters {
			name := lo.FromPtr(param.Name)
			if filterRegex != nil && !filterRegex.MatchString(name) {
				continue
			}
			params = append(params, paramInfo{
				Name: name,
				Type: string(param.Type),
			})
		}

		// Update next token
		nextToken = lo.FromPtr(page.NextToken)

		// Check if we have enough results or no more pages
		if len(params) >= input.MaxResults || nextToken == "" {
			break
		}
	}

	// Trim to MaxResults if we got more
	outputNextToken := nextToken
	if len(params) > input.MaxResults {
		params = params[:input.MaxResults]
		// Keep the nextToken so caller can fetch more
	}

	return u.buildOutput(ctx, input.WithValue, params, outputNextToken)
}

// buildOutput creates the output with optional values.
func (u *ListUseCase) buildOutput(ctx context.Context, withValue bool, params []paramInfo, nextToken string) (*ListOutput, error) {
	output := &ListOutput{NextToken: nextToken}

	// If values are not requested, return names and types only
	if !withValue {
		for _, p := range params {
			output.Entries = append(output.Entries, ListEntry{Name: p.Name, Type: p.Type})
		}
		return output, nil
	}

	// Fetch values in parallel
	namesMap := make(map[string]struct{}, len(params))
	for _, p := range params {
		namesMap[p.Name] = struct{}{}
	}

	results := parallel.ExecuteMap(ctx, namesMap, func(ctx context.Context, name string, _ struct{}) (string, error) {
		out, err := u.Client.GetParameter(ctx, &paramapi.GetParameterInput{
			Name:           lo.ToPtr(name),
			WithDecryption: lo.ToPtr(true),
		})
		if err != nil {
			return "", err
		}
		return lo.FromPtr(out.Parameter.Value), nil
	})

	// Collect results in order
	for _, p := range params {
		result := results[p.Name]
		entry := ListEntry{Name: p.Name, Type: p.Type}
		if result.Err != nil {
			entry.Error = result.Err
		} else {
			entry.Value = lo.ToPtr(result.Value)
		}
		output.Entries = append(output.Entries, entry)
	}

	return output, nil
}
