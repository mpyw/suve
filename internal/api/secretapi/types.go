package secretapi

import (
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// Client is a re-exported Secrets Manager client type for dependency injection.
type Client = secretsmanager.Client

// Options is a re-exported Secrets Manager options type.
type Options = secretsmanager.Options

// GetSecretValueInput is a re-exported Secrets Manager input type.
type GetSecretValueInput = secretsmanager.GetSecretValueInput

// GetSecretValueOutput is a re-exported Secrets Manager output type.
type GetSecretValueOutput = secretsmanager.GetSecretValueOutput

// ListSecretVersionIDsInput is a re-exported Secrets Manager input type.
type ListSecretVersionIDsInput = secretsmanager.ListSecretVersionIdsInput

// ListSecretVersionIDsOutput is a re-exported Secrets Manager output type.
type ListSecretVersionIDsOutput = secretsmanager.ListSecretVersionIdsOutput

// ListSecretsInput is a re-exported Secrets Manager input type.
type ListSecretsInput = secretsmanager.ListSecretsInput

// ListSecretsOutput is a re-exported Secrets Manager output type.
type ListSecretsOutput = secretsmanager.ListSecretsOutput

// CreateSecretInput is a re-exported Secrets Manager input type.
type CreateSecretInput = secretsmanager.CreateSecretInput

// CreateSecretOutput is a re-exported Secrets Manager output type.
type CreateSecretOutput = secretsmanager.CreateSecretOutput

// PutSecretValueInput is a re-exported Secrets Manager input type.
type PutSecretValueInput = secretsmanager.PutSecretValueInput

// PutSecretValueOutput is a re-exported Secrets Manager output type.
type PutSecretValueOutput = secretsmanager.PutSecretValueOutput

// DeleteSecretInput is a re-exported Secrets Manager input type.
type DeleteSecretInput = secretsmanager.DeleteSecretInput

// DeleteSecretOutput is a re-exported Secrets Manager output type.
type DeleteSecretOutput = secretsmanager.DeleteSecretOutput

// RestoreSecretInput is a re-exported Secrets Manager input type.
type RestoreSecretInput = secretsmanager.RestoreSecretInput

// RestoreSecretOutput is a re-exported Secrets Manager output type.
type RestoreSecretOutput = secretsmanager.RestoreSecretOutput

// UpdateSecretInput is a re-exported Secrets Manager input type.
type UpdateSecretInput = secretsmanager.UpdateSecretInput

// UpdateSecretOutput is a re-exported Secrets Manager output type.
type UpdateSecretOutput = secretsmanager.UpdateSecretOutput

// TagResourceInput is a re-exported Secrets Manager input type.
type TagResourceInput = secretsmanager.TagResourceInput

// TagResourceOutput is a re-exported Secrets Manager output type.
type TagResourceOutput = secretsmanager.TagResourceOutput

// UntagResourceInput is a re-exported Secrets Manager input type.
type UntagResourceInput = secretsmanager.UntagResourceInput

// UntagResourceOutput is a re-exported Secrets Manager output type.
type UntagResourceOutput = secretsmanager.UntagResourceOutput

// DescribeSecretInput is a re-exported Secrets Manager input type.
type DescribeSecretInput = secretsmanager.DescribeSecretInput

// DescribeSecretOutput is a re-exported Secrets Manager output type.
type DescribeSecretOutput = secretsmanager.DescribeSecretOutput

// SecretVersionsListEntry is a re-exported Secrets Manager model type.
type SecretVersionsListEntry = types.SecretVersionsListEntry

// SecretListEntry is a re-exported Secrets Manager model type.
type SecretListEntry = types.SecretListEntry

// Tag is a re-exported Secrets Manager model type.
type Tag = types.Tag

// Filter is a re-exported Secrets Manager model type.
type Filter = types.Filter

// FilterNameStringType is a re-exported Secrets Manager model type.
type FilterNameStringType = types.FilterNameStringType

// Re-exported Secrets Manager constants.
const (
	FilterNameStringTypeName = types.FilterNameStringTypeName
)

// ResourceNotFoundException is a re-exported Secrets Manager error type.
//
//nolint:errname // Type alias to AWS SDK type, preserving original name for consistency
type ResourceNotFoundException = types.ResourceNotFoundException

// NewFromConfig is a re-exported Secrets Manager factory function for dependency injection.
//
//nolint:gochecknoglobals // Re-export of AWS SDK factory function for dependency injection
var NewFromConfig = secretsmanager.NewFromConfig

// NewListSecretsPaginator is a re-exported Secrets Manager factory function for dependency injection.
//
//nolint:gochecknoglobals // Re-export of AWS SDK factory function for dependency injection
var NewListSecretsPaginator = secretsmanager.NewListSecretsPaginator
