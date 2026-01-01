// Package strategy provides SSM-specific stage command strategy implementations.
package strategy

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the combined interface for SSM stage operations.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
	ssmapi.PutParameterAPI
	ssmapi.DeleteParameterAPI
}

// Strategy implements stage.ServiceStrategy for SSM.
type Strategy struct {
	Client Client
}

// NewStrategy creates a new SSM strategy.
func NewStrategy(client Client) *Strategy {
	return &Strategy{Client: client}
}

// Service returns the service type.
func (s *Strategy) Service() stage.Service {
	return stage.ServiceSSM
}

// ServiceName returns the user-friendly service name.
func (s *Strategy) ServiceName() string {
	return "SSM"
}

// ItemName returns the item name for messages.
func (s *Strategy) ItemName() string {
	return "parameter"
}

// HasDeleteOptions returns false as SSM doesn't have delete options.
func (s *Strategy) HasDeleteOptions() bool {
	return false
}

// PushSet applies a set operation to AWS SSM.
func (s *Strategy) PushSet(ctx context.Context, name, value string) error {
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

	_, err = s.Client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      lo.ToPtr(name),
		Value:     lo.ToPtr(value),
		Type:      paramType,
		Overwrite: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}
	return nil
}

// PushDelete applies a delete operation to AWS SSM.
func (s *Strategy) PushDelete(ctx context.Context, name string, _ stage.Entry) error {
	_, err := s.Client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}
	return nil
}

// FetchCurrent fetches the current value from AWS SSM for diffing.
func (s *Strategy) FetchCurrent(ctx context.Context, name string) (*stage.FetchResult, error) {
	spec := &ssmversion.Spec{Name: name}
	param, err := ssmversion.GetParameterWithVersion(ctx, s.Client, spec, true)
	if err != nil {
		return nil, err
	}
	return &stage.FetchResult{
		Value:        lo.FromPtr(param.Value),
		VersionLabel: fmt.Sprintf("#%d", param.Version),
	}, nil
}

// ParseName parses and validates a name for editing.
func (s *Strategy) ParseName(input string) (string, error) {
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
func (s *Strategy) FetchCurrentValue(ctx context.Context, name string) (string, error) {
	spec := &ssmversion.Spec{Name: name}
	param, err := ssmversion.GetParameterWithVersion(ctx, s.Client, spec, true)
	if err != nil {
		return "", err
	}
	return lo.FromPtr(param.Value), nil
}

// ParseSpec parses a version spec string for reset.
func (s *Strategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := ssmversion.Parse(input)
	if err != nil {
		return "", false, err
	}
	hasVersion = spec.Absolute.Version != nil || spec.Shift > 0
	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *Strategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
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

// Factory creates a FullStrategy with an initialized AWS client.
func Factory(ctx context.Context) (stage.FullStrategy, error) {
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}
	return NewStrategy(client), nil
}

// FactoryWithoutClient creates a Strategy without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func FactoryWithoutClient() stage.FullStrategy {
	return NewStrategy(nil)
}
