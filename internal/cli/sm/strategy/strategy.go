// Package strategy provides SM-specific stage command strategy implementations.
package strategy

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/smutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the combined interface for SM stage operations.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
	smapi.PutSecretValueAPI
	smapi.DeleteSecretAPI
}

// Strategy implements stage.ServiceStrategy for SM.
type Strategy struct {
	Client Client
}

// NewStrategy creates a new SM strategy.
func NewStrategy(client Client) *Strategy {
	return &Strategy{Client: client}
}

// Service returns the service type.
func (s *Strategy) Service() stage.Service {
	return stage.ServiceSM
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

// PushSet applies a set operation to AWS Secrets Manager.
func (s *Strategy) PushSet(ctx context.Context, name, value string) error {
	_, err := s.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}

// PushDelete applies a delete operation to AWS Secrets Manager.
func (s *Strategy) PushDelete(ctx context.Context, name string, entry stage.Entry) error {
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
func (s *Strategy) FetchCurrent(ctx context.Context, name string) (*stage.FetchResult, error) {
	spec := &smversion.Spec{Name: name}
	secret, err := smversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return nil, err
	}
	versionID := smutil.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return &stage.FetchResult{
		Value:        lo.FromPtr(secret.SecretString),
		VersionLabel: "#" + versionID,
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
	versionID := smutil.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return lo.FromPtr(secret.SecretString), "#" + versionID, nil
}
