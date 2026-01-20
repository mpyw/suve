package model

import "time"

// ============================================================================
// Generic Secret (Provider Layer)
// ============================================================================

// TypedSecret is a secret with provider-specific metadata.
// This type is used at the Provider layer for type-safe access to metadata.
type TypedSecret[M any] struct {
	Name        string
	Value       string
	Version     string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    M
}

// ToBase converts to a UseCase layer type.
func (s *TypedSecret[M]) ToBase() *Secret {
	return &Secret{
		Name:        s.Name,
		Value:       s.Value,
		Version:     s.Version,
		Description: s.Description,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		Tags:        s.Tags,
		Metadata:    s.Metadata,
	}
}

// ============================================================================
// Base Secret (UseCase Layer)
// ============================================================================

// Secret is a provider-agnostic secret for the UseCase layer.
type Secret struct {
	Name        string
	Value       string
	Version     string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    any // Provider-specific metadata (e.g., AWSSecretMeta)
}

// AWSMeta returns the AWS-specific metadata if available.
func (s *Secret) AWSMeta() *AWSSecretMeta {
	if meta, ok := s.Metadata.(AWSSecretMeta); ok {
		return &meta
	}

	if meta, ok := s.Metadata.(*AWSSecretMeta); ok {
		return meta
	}

	return nil
}

// TypedSecretMetadata casts Metadata to a specific type.
func TypedSecretMetadata[M any](s *Secret) (M, bool) {
	m, ok := s.Metadata.(M)

	return m, ok
}

// ============================================================================
// Provider-Specific Metadata
// ============================================================================

// AWSSecretMeta contains AWS Secrets Manager-specific metadata.
type AWSSecretMeta struct {
	// ARN is the Amazon Resource Name of the secret.
	ARN string
	// VersionStages are the staging labels for this version.
	VersionStages []string
	// KmsKeyID is the ARN of the KMS key used for encryption.
	KmsKeyID string
	// RotationEnabled indicates if rotation is enabled.
	RotationEnabled bool
	// RotationRules contains rotation configuration.
	RotationRules *AWSRotationRules
	// DeletedDate is set if the secret is scheduled for deletion.
	DeletedDate *time.Time
}

// AWSRotationRules contains AWS secret rotation configuration.
type AWSRotationRules struct {
	AutomaticallyAfterDays int64
	Duration               string
	ScheduleExpression     string
}

// GCPSecretMeta contains GCP Secret Manager-specific metadata.
type GCPSecretMeta struct {
	// ReplicationPolicy describes how the secret is replicated.
	ReplicationPolicy string
	// State is the secret state (e.g., ENABLED, DISABLED).
	State string
	// Expiration is when the secret version expires.
	Expiration *time.Time
}

// AzureKeyVaultMeta contains Azure Key Vault-specific metadata.
type AzureKeyVaultMeta struct {
	// ContentType is the content type of the secret.
	ContentType string
	// Enabled indicates if the secret is enabled.
	Enabled bool
	// NotBefore is the earliest time the secret can be used.
	NotBefore *time.Time
	// Expiration is when the secret expires.
	Expiration *time.Time
	// RecoveryLevel is the deletion recovery level.
	RecoveryLevel string
}

// ============================================================================
// Type Aliases
// ============================================================================

// AWSSecret is a Secret with AWS-specific metadata.
type AWSSecret = TypedSecret[AWSSecretMeta]

// GCPSecret is a Secret with GCP-specific metadata.
type GCPSecret = TypedSecret[GCPSecretMeta]

// AzureSecret is a Secret with Azure-specific metadata.
type AzureSecret = TypedSecret[AzureKeyVaultMeta]

// ============================================================================
// Version Types
// ============================================================================

// TypedSecretVersion represents a version of a typed secret.
type TypedSecretVersion[M any] struct {
	Version   string
	CreatedAt *time.Time
	Metadata  M
}

// ToBase converts to a UseCase layer type.
func (v *TypedSecretVersion[M]) ToBase() *SecretVersion {
	return &SecretVersion{
		Version:   v.Version,
		CreatedAt: v.CreatedAt,
		Metadata:  v.Metadata,
	}
}

// SecretVersion represents a version of a secret.
type SecretVersion struct {
	Version   string
	CreatedAt *time.Time
	Metadata  any // Provider-specific metadata
}

// AWSSecretVersionMeta contains AWS-specific version metadata.
type AWSSecretVersionMeta struct {
	VersionStages []string
}

// AWSSecretVersion is a SecretVersion with AWS-specific metadata.
type AWSSecretVersion = TypedSecretVersion[AWSSecretVersionMeta]

// ============================================================================
// List Types
// ============================================================================

// SecretListItem represents a secret in a list (without value).
type SecretListItem struct {
	Name        string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    any // Provider-specific metadata (e.g., AWSSecretListItemMeta)
}

// AWSMeta returns the AWS-specific metadata if available.
func (s *SecretListItem) AWSMeta() *AWSSecretListItemMeta {
	if meta, ok := s.Metadata.(AWSSecretListItemMeta); ok {
		return &meta
	}

	if meta, ok := s.Metadata.(*AWSSecretListItemMeta); ok {
		return meta
	}

	return nil
}

// AWSSecretListItemMeta contains AWS Secrets Manager-specific metadata for list items.
type AWSSecretListItemMeta struct {
	// ARN is the Amazon Resource Name of the secret.
	ARN string
	// DeletedDate is set if the secret is scheduled for deletion.
	DeletedDate *time.Time
}

// ============================================================================
// Write Result Types
// ============================================================================

// SecretWriteResult contains the result of a secret write operation.
type SecretWriteResult struct {
	Name    string
	Version string
	ARN     string
}

// SecretDeleteResult contains the result of a secret delete operation.
type SecretDeleteResult struct {
	Name         string
	ARN          string
	DeletionDate *time.Time
}

// SecretRestoreResult contains the result of a secret restore operation.
type SecretRestoreResult struct {
	Name string
	ARN  string
}
