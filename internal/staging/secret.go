package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/provider"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// SecretClient is the combined interface for Secrets Manager stage operations.
type SecretClient interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIDsAPI
	secretapi.ListSecretsAPI
	secretapi.CreateSecretAPI
	secretapi.PutSecretValueAPI
	secretapi.DeleteSecretAPI
	secretapi.RestoreSecretAPI
	secretapi.UpdateSecretAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
	secretapi.DescribeSecretAPI
}

// SecretStrategy implements ServiceStrategy for Secrets Manager.
type SecretStrategy struct {
	Client SecretClient
	Reader provider.SecretReader // for version resolution
}

// NewSecretStrategy creates a new Secrets Manager strategy.
// Reader is automatically created from client for version resolution.
func NewSecretStrategy(client SecretClient) *SecretStrategy {
	s := &SecretStrategy{Client: client}
	if client != nil {
		s.Reader = awssecret.New(client)
	}

	return s
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

// HasDeleteOptions returns true as Secrets Manager has delete options.
func (s *SecretStrategy) HasDeleteOptions() bool {
	return true
}

// Apply applies a staged operation to AWS Secrets Manager.
func (s *SecretStrategy) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.applyCreate(ctx, name, entry)
	case OperationUpdate:
		return s.applyUpdate(ctx, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name, entry)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *SecretStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	input := &secretapi.CreateSecretInput{
		Name:         lo.ToPtr(name),
		SecretString: entry.Value,
	}
	if entry.Description != nil {
		input.Description = entry.Description
	}

	_, err := s.Client.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func (s *SecretStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	// Update secret value if provided
	if entry.Value != nil {
		_, err := s.Client.PutSecretValue(ctx, &secretapi.PutSecretValueInput{
			SecretId:     lo.ToPtr(name),
			SecretString: entry.Value,
		})
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	// Update description if provided
	if entry.Description != nil {
		_, err := s.Client.UpdateSecret(ctx, &secretapi.UpdateSecretInput{
			SecretId:    lo.ToPtr(name),
			Description: entry.Description,
		})
		if err != nil {
			return fmt.Errorf("failed to update description: %w", err)
		}
	}

	return nil
}

// ApplyTags applies staged tag changes to AWS Secrets Manager.
func (s *SecretStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
	change := &tagging.Change{
		Add:    tagEntry.Add,
		Remove: tagEntry.Remove,
	}
	if !change.IsEmpty() {
		if err := tagging.ApplySecret(ctx, s.Client, name, change); err != nil {
			return err
		}
	}

	return nil
}

func (s *SecretStrategy) applyDelete(ctx context.Context, name string, entry Entry) error {
	input := &secretapi.DeleteSecretInput{
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
		if rnf := (*secretapi.ResourceNotFoundException)(nil); errors.As(err, &rnf) {
			return nil
		}

		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// FetchLastModified returns the last modified time of the secret in AWS.
// Returns zero time if the secret doesn't exist.
func (s *SecretStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	result, err := s.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
		SecretId: lo.ToPtr(name),
	})
	if err != nil {
		if rnf := (*secretapi.ResourceNotFoundException)(nil); errors.As(err, &rnf) {
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

	secret, err := secretversion.GetSecretWithVersion(ctx, s.Reader, spec)
	if err != nil {
		return nil, err
	}

	versionID := secretversion.TruncateVersionID(secret.Version)

	return &FetchResult{
		Value:      secret.Value,
		Identifier: "#" + versionID,
	}, nil
}

// FetchCurrentTags fetches the current tags from AWS Secrets Manager.
func (s *SecretStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	result, err := s.Client.DescribeSecret(ctx, &secretapi.DescribeSecretInput{
		SecretId: lo.ToPtr(name),
	})
	if err != nil {
		// Secret not found - return nil (no tags available)
		if rnf := (*secretapi.ResourceNotFoundException)(nil); errors.As(err, &rnf) {
			return nil, nil //nolint:nilnil // intentional: no tags for non-existent resource
		}

		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	if result == nil || len(result.Tags) == 0 {
		return nil, nil //nolint:nilnil // intentional: resource exists but has no tags
	}

	tags := make(map[string]string, len(result.Tags))
	for _, tag := range result.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	return tags, nil
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
// Returns *ResourceNotFoundError if the secret doesn't exist.
func (s *SecretStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	spec := &secretversion.Spec{Name: name}

	secret, err := secretversion.GetSecretWithVersion(ctx, s.Reader, spec)
	if err != nil {
		if rnf := (*secretapi.ResourceNotFoundException)(nil); errors.As(err, &rnf) {
			return nil, &ResourceNotFoundError{Err: err}
		}

		return nil, err
	}

	result := &EditFetchResult{
		Value: secret.Value,
	}
	if secret.CreatedAt != nil {
		result.LastModified = *secret.CreatedAt
	}

	return result, nil
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

	secret, err := secretversion.GetSecretWithVersion(ctx, s.Reader, spec)
	if err != nil {
		return "", "", err
	}

	versionID := secretversion.TruncateVersionID(secret.Version)

	return secret.Value, "#" + versionID, nil
}

// SecretFactory creates a FullStrategy with an initialized AWS client.
func SecretFactory(ctx context.Context) (FullStrategy, error) {
	// Create raw client for apply operations (secretapi interface)
	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// Create adapter for version resolution (provider interface)
	adapter := awssecret.New(client)

	return &SecretStrategy{
		Client: client,
		Reader: adapter,
	}, nil
}

// SecretParserFactory creates a Parser without an AWS client.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func SecretParserFactory() Parser {
	return NewSecretStrategy(nil)
}
