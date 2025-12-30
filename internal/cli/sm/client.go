// Package sm provides CLI commands for AWS Secrets Manager.
package sm

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

// RestoreSecretAPI is the interface for restoring a secret.
type RestoreSecretAPI interface {
	RestoreSecret(ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error)
}

// ShowClient is the interface for the show command.
type ShowClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// CatClient is the interface for the cat command.
type CatClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// LogClient is the interface for the log command.
type LogClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// DiffClient is the interface for the diff command.
type DiffClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// LsClient is the interface for the ls command.
type LsClient interface {
	ListSecretsAPI
}

// CreateClient is the interface for the create command.
type CreateClient interface {
	CreateSecretAPI
}

// SetClient is the interface for the set command.
type SetClient interface {
	PutSecretValueAPI
}

// RmClient is the interface for the rm command.
type RmClient interface {
	DeleteSecretAPI
}

// RestoreClient is the interface for the restore command.
type RestoreClient interface {
	RestoreSecretAPI
}
