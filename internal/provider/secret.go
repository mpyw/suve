package provider

import (
	"context"

	"github.com/mpyw/suve/internal/model"
)

// ============================================================================
// UseCase Layer Interfaces
// ============================================================================

// SecretReader provides read access to secrets.
type SecretReader interface {
	// GetSecret retrieves a secret by name with optional version/stage specifier.
	// - versionID: specific version ID (empty for latest)
	// - versionStage: staging label like "AWSCURRENT" (empty to ignore)
	GetSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error)

	// GetSecretVersions retrieves all versions of a secret.
	GetSecretVersions(ctx context.Context, name string) ([]*model.SecretVersion, error)

	// ListSecrets lists all secrets.
	ListSecrets(ctx context.Context) ([]*model.SecretListItem, error)
}

// SecretWriter provides write access to secrets.
type SecretWriter interface {
	// CreateSecret creates a new secret.
	CreateSecret(ctx context.Context, secret *model.Secret) error

	// UpdateSecret updates the value of an existing secret.
	UpdateSecret(ctx context.Context, name string, value string) error

	// DeleteSecret deletes a secret.
	// If forceDelete is true, immediately deletes without recovery window.
	DeleteSecret(ctx context.Context, name string, forceDelete bool) error
}

// SecretTagger provides tag management for secrets.
//
//nolint:iface // Intentionally similar to ParameterTagger but separate for clarity.
type SecretTagger interface {
	// GetTags retrieves all tags for a secret.
	GetTags(ctx context.Context, name string) (map[string]string, error)

	// AddTags adds or updates tags on a secret.
	AddTags(ctx context.Context, name string, tags map[string]string) error

	// RemoveTags removes tags from a secret by key names.
	RemoveTags(ctx context.Context, name string, keys []string) error
}

// SecretService combines all secret operations.
type SecretService interface {
	SecretReader
	SecretWriter
	SecretTagger
}

// ============================================================================
// Provider-Specific Extensions (Optional Interfaces)
// ============================================================================

// SecretRestorer provides secret restoration capability.
// This is an optional interface for providers that support restoring deleted secrets.
type SecretRestorer interface {
	// RestoreSecret restores a previously deleted secret.
	RestoreSecret(ctx context.Context, name string) error
}

// SecretDescriber provides secret metadata without the value.
// This is an optional interface for providers that support separate describe operation.
type SecretDescriber interface {
	// DescribeSecret retrieves secret metadata without the value.
	DescribeSecret(ctx context.Context, name string) (*model.SecretListItem, error)
}

// ============================================================================
// Provider Layer Interfaces (Generic)
// ============================================================================

// TypedSecretReader provides type-safe access to secrets with metadata.
// This is used internally by provider adapters.
type TypedSecretReader[M any] interface {
	// GetTypedSecret retrieves a secret with provider-specific metadata.
	GetTypedSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.TypedSecret[M], error)
}

// ============================================================================
// Adapter Helpers
// ============================================================================

// PartialSecretReader wraps a TypedSecretReader to provide GetSecret method.
type PartialSecretReader[M any] struct {
	inner TypedSecretReader[M]
}

// WrapTypedSecretReader wraps a TypedSecretReader to implement partial SecretReader.
func WrapTypedSecretReader[M any](r TypedSecretReader[M]) *PartialSecretReader[M] {
	return &PartialSecretReader[M]{inner: r}
}

// GetSecret retrieves a secret and converts it to the base type.
func (a *PartialSecretReader[M]) GetSecret(
	ctx context.Context, name string, versionID string, versionStage string,
) (*model.Secret, error) {
	s, err := a.inner.GetTypedSecret(ctx, name, versionID, versionStage)
	if err != nil {
		return nil, err
	}

	return s.ToBase(), nil
}
