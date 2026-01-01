package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// SSMClient is the combined interface for SSM stage operations.
type SSMClient interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
	ssmapi.PutParameterAPI
	ssmapi.DeleteParameterAPI
	ssmapi.AddTagsToResourceAPI
	ssmapi.RemoveTagsFromResourceAPI
}

// SSMStrategy implements ServiceStrategy for SSM Parameter Store.
type SSMStrategy struct {
	Client SSMClient
}

// NewSSMStrategy creates a new SSM strategy.
func NewSSMStrategy(client SSMClient) *SSMStrategy {
	return &SSMStrategy{Client: client}
}

// Service returns the service type.
func (s *SSMStrategy) Service() Service {
	return ServiceSSM
}

// ServiceName returns the user-friendly service name.
func (s *SSMStrategy) ServiceName() string {
	return "SSM"
}

// ItemName returns the item name for messages.
func (s *SSMStrategy) ItemName() string {
	return "parameter"
}

// HasDeleteOptions returns false as SSM doesn't have delete options.
func (s *SSMStrategy) HasDeleteOptions() bool {
	return false
}

// Push applies a staged operation to AWS SSM.
func (s *SSMStrategy) Push(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		return s.pushSet(ctx, name, entry)
	case OperationDelete:
		return s.pushDelete(ctx, name)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *SSMStrategy) pushSet(ctx context.Context, name string, entry Entry) error {
	// Try to get existing parameter to preserve type
	paramType := types.ParameterTypeString
	existing, err := s.Client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		var pnf *types.ParameterNotFound
		if !errors.As(err, &pnf) {
			return fmt.Errorf("failed to get existing parameter: %w", err)
		}
	} else if existing.Parameter != nil {
		paramType = existing.Parameter.Type
	}

	input := &ssm.PutParameterInput{
		Name:      lo.ToPtr(name),
		Value:     lo.ToPtr(entry.Value),
		Type:      paramType,
		Overwrite: lo.ToPtr(true),
	}
	if entry.Description != nil {
		input.Description = entry.Description
	}

	_, err = s.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	// Apply tag changes (additive)
	if len(entry.Tags) > 0 || len(entry.UntagKeys) > 0 {
		change := &tagging.Change{
			Add:    entry.Tags,
			Remove: entry.UntagKeys,
		}
		if !change.IsEmpty() {
			if err := tagging.ApplySSM(ctx, s.Client, name, change); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SSMStrategy) pushDelete(ctx context.Context, name string) error {
	_, err := s.Client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		// Already deleted is considered success
		var pnf *types.ParameterNotFound
		if errors.As(err, &pnf) {
			return nil
		}
		return fmt.Errorf("failed to delete parameter: %w", err)
	}
	return nil
}

// FetchLastModified returns the last modified time of the parameter in AWS.
// Returns zero time if the parameter doesn't exist.
func (s *SSMStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	result, err := s.Client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		var pnf *types.ParameterNotFound
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

// FetchCurrent fetches the current value from AWS SSM for diffing.
func (s *SSMStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	spec := &ssmversion.Spec{Name: name}
	param, err := ssmversion.GetParameterWithVersion(ctx, s.Client, spec, true)
	if err != nil {
		return nil, err
	}
	return &FetchResult{
		Value:      lo.FromPtr(param.Value),
		Identifier: fmt.Sprintf("#%d", param.Version),
	}, nil
}

// ParseName parses and validates a name for editing.
func (s *SSMStrategy) ParseName(input string) (string, error) {
	spec, err := ssmversion.Parse(input)
	if err != nil {
		return "", err
	}
	if spec.Absolute.Version != nil || spec.Shift > 0 {
		return "", fmt.Errorf("stage diff requires a parameter name without version specifier")
	}
	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from AWS SSM for editing.
func (s *SSMStrategy) FetchCurrentValue(ctx context.Context, name string) (string, error) {
	spec := &ssmversion.Spec{Name: name}
	param, err := ssmversion.GetParameterWithVersion(ctx, s.Client, spec, true)
	if err != nil {
		return "", err
	}
	return lo.FromPtr(param.Value), nil
}

// ParseSpec parses a version spec string for reset.
func (s *SSMStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := ssmversion.Parse(input)
	if err != nil {
		return "", false, err
	}
	hasVersion = spec.Absolute.Version != nil || spec.Shift > 0
	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *SSMStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := ssmversion.Parse(input)
	if err != nil {
		return "", "", err
	}
	param, err := ssmversion.GetParameterWithVersion(ctx, s.Client, spec, true)
	if err != nil {
		return "", "", err
	}
	return lo.FromPtr(param.Value), fmt.Sprintf("#%d", param.Version), nil
}

// SSMFactory creates a FullStrategy with an initialized AWS client.
func SSMFactory(ctx context.Context) (FullStrategy, error) {
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}
	return NewSSMStrategy(client), nil
}

// SSMParserFactory creates a Parser without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func SSMParserFactory() Parser {
	return NewSSMStrategy(nil)
}
