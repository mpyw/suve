package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// ParamClient is the combined interface for SSM Parameter Store stage operations.
type ParamClient interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
	paramapi.PutParameterAPI
	paramapi.DeleteParameterAPI
	paramapi.AddTagsToResourceAPI
	paramapi.RemoveTagsFromResourceAPI
}

// ParamStrategy implements ServiceStrategy for SSM Parameter Store.
type ParamStrategy struct {
	Client ParamClient
}

// NewParamStrategy creates a new SSM Parameter Store strategy.
func NewParamStrategy(client ParamClient) *ParamStrategy {
	return &ParamStrategy{Client: client}
}

// Service returns the service type.
func (s *ParamStrategy) Service() Service {
	return ServiceParam
}

// ServiceName returns the user-friendly service name.
func (s *ParamStrategy) ServiceName() string {
	return "SSM Parameter Store"
}

// ItemName returns the item name for messages.
func (s *ParamStrategy) ItemName() string {
	return "parameter"
}

// HasDeleteOptions returns false as SSM Parameter Store doesn't have delete options.
func (s *ParamStrategy) HasDeleteOptions() bool {
	return false
}

// Apply applies a staged operation to AWS SSM Parameter Store.
func (s *ParamStrategy) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.applyCreate(ctx, name, entry)
	case OperationUpdate:
		return s.applyUpdate(ctx, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *ParamStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	input := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(name),
		Value:     entry.Value,
		Type:      paramapi.ParameterTypeString,
		Overwrite: lo.ToPtr(false), // Do not overwrite existing parameters
	}
	if entry.Description != nil {
		input.Description = entry.Description
	}

	_, err := s.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create parameter: %w", err)
	}

	return nil
}

func (s *ParamStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Check if parameter exists
	existing, err := s.Client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		var pnf *paramapi.ParameterNotFound
		if errors.As(err, &pnf) {
			return fmt.Errorf("parameter not found: %s", name)
		}
		return fmt.Errorf("failed to get existing parameter: %w", err)
	}

	// Preserve existing parameter type
	paramType := paramapi.ParameterTypeString
	if existing.Parameter != nil {
		paramType = existing.Parameter.Type
	}

	input := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(name),
		Value:     entry.Value,
		Type:      paramType,
		Overwrite: lo.ToPtr(true),
	}
	if entry.Description != nil {
		input.Description = entry.Description
	}

	_, err = s.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update parameter: %w", err)
	}

	return nil
}

// ApplyTags applies staged tag changes to AWS SSM Parameter Store.
func (s *ParamStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
	change := &tagging.Change{
		Add:    tagEntry.Add,
		Remove: tagEntry.Remove,
	}
	if !change.IsEmpty() {
		if err := tagging.ApplyParam(ctx, s.Client, name, change); err != nil {
			return err
		}
	}
	return nil
}

func (s *ParamStrategy) applyDelete(ctx context.Context, name string) error {
	_, err := s.Client.DeleteParameter(ctx, &paramapi.DeleteParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		// Already deleted is considered success
		var pnf *paramapi.ParameterNotFound
		if errors.As(err, &pnf) {
			return nil
		}
		return fmt.Errorf("failed to delete parameter: %w", err)
	}
	return nil
}

// FetchLastModified returns the last modified time of the parameter in AWS.
// Returns zero time if the parameter doesn't exist.
func (s *ParamStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	result, err := s.Client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		var pnf *paramapi.ParameterNotFound
		if errors.As(err, &pnf) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("failed to get parameter: %w", err)
	}
	if result.Parameter != nil && result.Parameter.LastModifiedDate != nil {
		return *result.Parameter.LastModifiedDate, nil
	}
	return time.Time{}, nil
}

// FetchCurrent fetches the current value from AWS SSM Parameter Store for diffing.
func (s *ParamStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	spec := &paramversion.Spec{Name: name}
	param, err := paramversion.GetParameterWithVersion(ctx, s.Client, spec)
	if err != nil {
		return nil, err
	}
	return &FetchResult{
		Value:      lo.FromPtr(param.Value),
		Identifier: fmt.Sprintf("#%d", param.Version),
	}, nil
}

// ParseName parses and validates a name for editing.
func (s *ParamStrategy) ParseName(input string) (string, error) {
	spec, err := paramversion.Parse(input)
	if err != nil {
		return "", err
	}
	if spec.Absolute.Version != nil || spec.Shift > 0 {
		return "", fmt.Errorf("stage diff requires a parameter name without version specifier")
	}
	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from AWS SSM Parameter Store for editing.
// Returns *ResourceNotFoundError if the parameter doesn't exist.
func (s *ParamStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	spec := &paramversion.Spec{Name: name}
	param, err := paramversion.GetParameterWithVersion(ctx, s.Client, spec)
	if err != nil {
		var pnf *paramapi.ParameterNotFound
		if errors.As(err, &pnf) {
			return nil, &ResourceNotFoundError{Err: err}
		}
		return nil, err
	}
	result := &EditFetchResult{
		Value: lo.FromPtr(param.Value),
	}
	if param.LastModifiedDate != nil {
		result.LastModified = *param.LastModifiedDate
	}
	return result, nil
}

// ParseSpec parses a version spec string for reset.
func (s *ParamStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := paramversion.Parse(input)
	if err != nil {
		return "", false, err
	}
	hasVersion = spec.Absolute.Version != nil || spec.Shift > 0
	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *ParamStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := paramversion.Parse(input)
	if err != nil {
		return "", "", err
	}
	param, err := paramversion.GetParameterWithVersion(ctx, s.Client, spec)
	if err != nil {
		return "", "", err
	}
	return lo.FromPtr(param.Value), fmt.Sprintf("#%d", param.Version), nil
}

// ParamFactory creates a FullStrategy with an initialized AWS client.
func ParamFactory(ctx context.Context) (FullStrategy, error) {
	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}
	return NewParamStrategy(client), nil
}

// ParamParserFactory creates a Parser without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func ParamParserFactory() Parser {
	return NewParamStrategy(nil)
}
