// Package sm provides SM-specific stage command strategy implementations.
package sm

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the combined interface for SM stage operations.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
	smapi.CreateSecretAPI
	smapi.PutSecretValueAPI
	smapi.DeleteSecretAPI
}

// Strategy implements staging.ServiceStrategy for SM.
type Strategy struct {
	Client Client
}

// NewStrategy creates a new SM strategy.
func NewStrategy(client Client) *Strategy {
	return &Strategy{Client: client}
}

// Service returns the service type.
func (s *Strategy) Service() staging.Service {
	return staging.ServiceSM
}

// ServiceName returns the user-friendly service name.
func (s *Strategy) ServiceName() string {
	return "SM"
}

// ItemName returns the item name for messages.
func (s *Strategy) ItemName() string {
	return "secret"
}

// HasDeleteOptions returns true as SM has delete options.
func (s *Strategy) HasDeleteOptions() bool {
	return true
}

// Push applies a staged operation to AWS Secrets Manager.
func (s *Strategy) Push(ctx context.Context, name string, entry staging.Entry) error {
	switch entry.Operation {
	case staging.OperationCreate:
		return s.pushCreate(ctx, name, entry.Value)
	case staging.OperationUpdate:
		return s.pushUpdate(ctx, name, entry.Value)
	case staging.OperationDelete:
		return s.pushDelete(ctx, name, entry)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *Strategy) pushCreate(ctx context.Context, name, value string) error {
	_, err := s.Client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	})
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}
	return nil
}

func (s *Strategy) pushUpdate(ctx context.Context, name, value string) error {
	_, err := s.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}

func (s *Strategy) pushDelete(ctx context.Context, name string, entry staging.Entry) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: lo.ToPtr(name),
	}

	if entry.DeleteOptions != nil {
		if entry.DeleteOptions.Force {
			input.ForceDeleteWithoutRecovery = lo.ToPtr(true)
		} else if entry.DeleteOptions.RecoveryWindow > 0 {
			input.RecoveryWindowInDays = lo.ToPtr(int64(entry.DeleteOptions.RecoveryWindow))
		}
	}

	_, err := s.Client.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// FetchCurrent fetches the current value from AWS Secrets Manager for diffing.
func (s *Strategy) FetchCurrent(ctx context.Context, name string) (*staging.FetchResult, error) {
	spec := &smversion.Spec{Name: name}
	secret, err := smversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return nil, err
	}
	versionID := smversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return &staging.FetchResult{
		Value:      lo.FromPtr(secret.SecretString),
		Identifier: "#" + versionID,
	}, nil
}

// ParseName parses and validates a name for editing.
func (s *Strategy) ParseName(input string) (string, error) {
	spec, err := smversion.Parse(input)
	if err != nil {
		return "", err
	}
	if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
		return "", fmt.Errorf("stage diff requires a secret name without version specifier")
	}
	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from AWS Secrets Manager for editing.
func (s *Strategy) FetchCurrentValue(ctx context.Context, name string) (string, error) {
	spec := &smversion.Spec{Name: name}
	secret, err := smversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return "", err
	}
	return lo.FromPtr(secret.SecretString), nil
}

// ParseSpec parses a version spec string for reset.
func (s *Strategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := smversion.Parse(input)
	if err != nil {
		return "", false, err
	}
	hasVersion = spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0
	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *Strategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := smversion.Parse(input)
	if err != nil {
		return "", "", err
	}
	secret, err := smversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return "", "", err
	}
	versionID := smversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return lo.FromPtr(secret.SecretString), "#" + versionID, nil
}

// Factory creates a FullStrategy with an initialized AWS client.
func Factory(ctx context.Context) (staging.FullStrategy, error) {
	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}
	return NewStrategy(client), nil
}

// ParserFactory creates a Parser without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func ParserFactory() staging.Parser {
	return NewStrategy(nil)
}
