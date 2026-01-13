package param

import (
	"context"
	"fmt"
	"regexp"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/parallel"
)

const (
	// getParametersBatchSize is the maximum number of parameters per GetParameters API call.
	// AWS SSM GetParameters API allows up to 10 parameters per request.
	getParametersBatchSize = 10

	// describeParametersMaxResults is the maximum results per DescribeParameters API call.
	// AWS SSM DescribeParameters API allows up to 50 parameters per request.
	describeParametersMaxResults int32 = 50
)

// ListClient is the interface for the list use case.
type ListClient interface {
	paramapi.DescribeParametersAPI
	paramapi.GetParametersAPI
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

	for {
		// Set pagination params
		apiInput.MaxResults = lo.ToPtr(describeParametersMaxResults)
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

	// Fetch values using batched GetParameters calls
	values, errs := u.fetchValuesBatched(ctx, params)

	// Collect results in order
	for _, p := range params {
		entry := ListEntry{Name: p.Name, Type: p.Type}

		if err, hasErr := errs[p.Name]; hasErr {
			entry.Error = err
		} else if val, hasVal := values[p.Name]; hasVal {
			entry.Value = lo.ToPtr(val)
		}

		output.Entries = append(output.Entries, entry)
	}

	return output, nil
}

// fetchValuesBatched fetches parameter values in batches using GetParameters API.
// Returns maps of name->value and name->error.
func (u *ListUseCase) fetchValuesBatched(ctx context.Context, params []paramInfo) (map[string]string, map[string]error) {
	if len(params) == 0 {
		return nil, nil
	}

	// Extract names and create batches using lo.Chunk
	names := lo.Map(params, func(p paramInfo, _ int) string { return p.Name })
	batches := lo.Chunk(names, getParametersBatchSize)

	// Create batch map for parallel execution
	batchMap := lo.SliceToMap(lo.Range(len(batches)), func(i int) (int, []string) {
		return i, batches[i]
	})

	// Execute batches in parallel
	results := parallel.ExecuteMap(ctx, batchMap, func(ctx context.Context, _ int, names []string) (*paramapi.GetParametersOutput, error) {
		return u.Client.GetParameters(ctx, &paramapi.GetParametersInput{
			Names:          names,
			WithDecryption: lo.ToPtr(true),
		})
	})

	// Collect results
	values := make(map[string]string)
	errs := make(map[string]error)

	for batchIdx, result := range results {
		if result.Err != nil {
			// If the entire batch failed, mark all parameters in that batch with the error
			for _, name := range batchMap[batchIdx] {
				errs[name] = fmt.Errorf("failed to get parameter: %w", result.Err)
			}

			continue
		}

		// Map successful parameters
		for _, param := range result.Value.Parameters {
			values[lo.FromPtr(param.Name)] = lo.FromPtr(param.Value)
		}

		// Map invalid parameters (not found)
		for _, name := range result.Value.InvalidParameters {
			errs[name] = fmt.Errorf("%w: %s", ErrParameterNotFound, name)
		}
	}

	return values, errs
}
