// Package secretapi provides interfaces for AWS Secrets Manager.
package secretapi

import (
	"context"
)

// GetSecretValueAPI is the interface for getting a secret value.
type GetSecretValueAPI interface {
	GetSecretValue(ctx context.Context, params *GetSecretValueInput, optFns ...func(*Options)) (*GetSecretValueOutput, error)
}

// ListSecretVersionIdsAPI is the interface for listing secret versions.
type ListSecretVersionIdsAPI interface {
	ListSecretVersionIds(ctx context.Context, params *ListSecretVersionIdsInput, optFns ...func(*Options)) (*ListSecretVersionIdsOutput, error)
}

// ListSecretsAPI is the interface for listing secrets.
type ListSecretsAPI interface {
	ListSecrets(ctx context.Context, params *ListSecretsInput, optFns ...func(*Options)) (*ListSecretsOutput, error)
}

// CreateSecretAPI is the interface for creating a secret.
type CreateSecretAPI interface {
	CreateSecret(ctx context.Context, params *CreateSecretInput, optFns ...func(*Options)) (*CreateSecretOutput, error)
}

// PutSecretValueAPI is the interface for updating a secret value.
type PutSecretValueAPI interface {
	PutSecretValue(ctx context.Context, params *PutSecretValueInput, optFns ...func(*Options)) (*PutSecretValueOutput, error)
}

// DeleteSecretAPI is the interface for deleting a secret.
type DeleteSecretAPI interface {
	DeleteSecret(ctx context.Context, params *DeleteSecretInput, optFns ...func(*Options)) (*DeleteSecretOutput, error)
}

// RestoreSecretAPI is the interface for restoring a deleted secret.
type RestoreSecretAPI interface {
	RestoreSecret(ctx context.Context, params *RestoreSecretInput, optFns ...func(*Options)) (*RestoreSecretOutput, error)
}

// UpdateSecretAPI is the interface for updating secret metadata (description).
type UpdateSecretAPI interface {
	UpdateSecret(ctx context.Context, params *UpdateSecretInput, optFns ...func(*Options)) (*UpdateSecretOutput, error)
}

// TagResourceAPI is the interface for tagging a secret.
type TagResourceAPI interface {
	TagResource(ctx context.Context, params *TagResourceInput, optFns ...func(*Options)) (*TagResourceOutput, error)
}

// UntagResourceAPI is the interface for removing tags from a secret.
type UntagResourceAPI interface {
	UntagResource(ctx context.Context, params *UntagResourceInput, optFns ...func(*Options)) (*UntagResourceOutput, error)
}

// DescribeSecretAPI is the interface for getting secret metadata including tags.
type DescribeSecretAPI interface {
	DescribeSecret(ctx context.Context, params *DescribeSecretInput, optFns ...func(*Options)) (*DescribeSecretOutput, error)
}
