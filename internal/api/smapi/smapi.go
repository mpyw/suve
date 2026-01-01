// Package smapi provides interfaces for AWS Secrets Manager.
package smapi

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// GetSecretValueAPI is the interface for getting a secret value.
type GetSecretValueAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// ListSecretVersionIdsAPI is the interface for listing secret versions.
type ListSecretVersionIdsAPI interface {
	ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

// ListSecretsAPI is the interface for listing secrets.
type ListSecretsAPI interface {
	ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

// CreateSecretAPI is the interface for creating a secret.
type CreateSecretAPI interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
}

// PutSecretValueAPI is the interface for updating a secret value.
type PutSecretValueAPI interface {
	PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
}

// DeleteSecretAPI is the interface for deleting a secret.
type DeleteSecretAPI interface {
	DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

// RestoreSecretAPI is the interface for restoring a deleted secret.
type RestoreSecretAPI interface {
	RestoreSecret(ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error)
}

// UpdateSecretAPI is the interface for updating secret metadata (description).
type UpdateSecretAPI interface {
	UpdateSecret(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error)
}

// TagResourceAPI is the interface for tagging a secret.
type TagResourceAPI interface {
	TagResource(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
}
