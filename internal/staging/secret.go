package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// SecretClient is the combined interface for SM stage operations.
type SecretClient interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIdsAPI
	secretapi.CreateSecretAPI
	secretapi.PutSecretValueAPI
	secretapi.DeleteSecretAPI
	secretapi.UpdateSecretAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
}

// SecretStrategy implements ServiceStrategy for Secrets Manager.
type SecretStrategy struct {
	Client SecretClient
}

// NewSecretStrategy creates a new SM strategy.
func NewSecretStrategy(client SecretClient) *SecretStrategy {
	return &SecretStrategy{Client: client}
}

// Service returns the service type.
func (s *SecretStrategy) Service() Service {
	return ServiceSecret
}

// ServiceName returns the user-friendly service name.
func (s *SecretStrategy) ServiceName() string {
	return "Secrets Manager"
}

// ItemName returns the item name for messages.
func (s *SecretStrategy) ItemName() string {
	return "secret"
}

// HasDeleteOptions returns true as SM has delete options.
func (s *SecretStrategy) HasDeleteOptions() bool {
	return true
}

// Push applies a staged operation to AWS Secrets Manager.
func (s *SecretStrategy) Push(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.pushCreate(ctx, name, entry)
	case OperationUpdate:
		return s.pushUpdate(ctx, name, entry)
	case OperationDelete:
		return s.pushDelete(ctx, name, entry)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *SecretStrategy) pushCreate(ctx context.Context, name string, entry Entry) error {
	input := &secretsmanager.CreateSecretInput{
		Name:         lo.ToPtr(name),
		SecretString: lo.ToPtr(entry.Value),
	}
	if entry.Description != nil {
		input.Description = entry.Description
	}
	if len(entry.Tags) > 0 {
		input.Tags = make([]types.Tag, 0, len(entry.Tags))
		for k, v := range entry.Tags {
			input.Tags = append(input.Tags, types.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
	}

	_, err := s.Client.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}
	return nil
}

func (s *SecretStrategy) pushUpdate(ctx context.Context, name string, entry Entry) error {
	// Update secret value
	_, err := s.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(entry.Value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	// Update description if provided
	if entry.Description != nil {
		_, err := s.Client.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
			SecretId:    lo.ToPtr(name),
			Description: entry.Description,
		})
		if err != nil {
			return fmt.Errorf("failed to update description: %w", err)
		}
	}

	// Apply tag changes (additive)
	if len(entry.Tags) > 0 || len(entry.UntagKeys) > 0 {
		change := &tagging.Change{
			Add:    entry.Tags,
			Remove: entry.UntagKeys,
		}
		if !change.IsEmpty() {
			if err := tagging.ApplySM(ctx, s.Client, name, change); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SecretStrategy) pushDelete(ctx context.Context, name string, entry Entry) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: lo.ToPtr(name),
	}

	if entry.DeleteOptions != nil {
		switch {
		case entry.DeleteOptions.Force:
			input.ForceDeleteWithoutRecovery = lo.ToPtr(true)
		case entry.DeleteOptions.RecoveryWindow > 0:
			input.RecoveryWindowInDays = lo.ToPtr(int64(entry.DeleteOptions.RecoveryWindow))
		}
	}

	_, err := s.Client.DeleteSecret(ctx, input)
	if err != nil {
		// Already deleted is considered success
		var rnf *types.ResourceNotFoundException
		if errors.As(err, &rnf) {
			return nil
		}
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// FetchLastModified returns the last modified time of the secret in AWS.
// Returns zero time if the secret doesn't exist.
func (s *SecretStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	result, err := s.Client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: lo.ToPtr(name),
	})
	if err != nil {
		var rnf *types.ResourceNotFoundException
		if errors.As(err, &rnf) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("failed to get secret: %w", err)
	}
	if result.CreatedDate != nil {
		return *result.CreatedDate, nil
	}
	return time.Time{}, nil
}

// FetchCurrent fetches the current value from AWS Secrets Manager for diffing.
func (s *SecretStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	spec := &secretversion.Spec{Name: name}
	secret, err := secretversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return nil, err
	}
	versionID := secretversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return &FetchResult{
		Value:      lo.FromPtr(secret.SecretString),
		Identifier: "#" + versionID,
	}, nil
}

// ParseName parses and validates a name for editing.
func (s *SecretStrategy) ParseName(input string) (string, error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", err
	}
	if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
		return "", fmt.Errorf("stage diff requires a secret name without version specifier")
	}
	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from AWS Secrets Manager for editing.
func (s *SecretStrategy) FetchCurrentValue(ctx context.Context, name string) (string, error) {
	spec := &secretversion.Spec{Name: name}
	secret, err := secretversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return "", err
	}
	return lo.FromPtr(secret.SecretString), nil
}

// ParseSpec parses a version spec string for reset.
func (s *SecretStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", false, err
	}
	hasVersion = spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0
	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *SecretStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", "", err
	}
	secret, err := secretversion.GetSecretWithVersion(ctx, s.Client, spec)
	if err != nil {
		return "", "", err
	}
	versionID := secretversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
	return lo.FromPtr(secret.SecretString), "#" + versionID, nil
}

// SecretFactory creates a FullStrategy with an initialized AWS client.
func SecretFactory(ctx context.Context) (FullStrategy, error) {
	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}
	return NewSecretStrategy(client), nil
}

// SecretParserFactory creates a Parser without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func SecretParserFactory() Parser {
	return NewSecretStrategy(nil)
}
