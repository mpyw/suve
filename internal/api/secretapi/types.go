// Package secretapi provides interfaces and types for AWS Secrets Manager.
package secretapi

import (
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// Re-exported Secrets Manager client and options types.
type (
	Client  = secretsmanager.Client
	Options = secretsmanager.Options
)

// Re-exported Secrets Manager input/output types.
type (
	GetSecretValueInput        = secretsmanager.GetSecretValueInput
	GetSecretValueOutput       = secretsmanager.GetSecretValueOutput
	ListSecretVersionIdsInput  = secretsmanager.ListSecretVersionIdsInput
	ListSecretVersionIdsOutput = secretsmanager.ListSecretVersionIdsOutput
	ListSecretsInput           = secretsmanager.ListSecretsInput
	ListSecretsOutput          = secretsmanager.ListSecretsOutput
	CreateSecretInput          = secretsmanager.CreateSecretInput
	CreateSecretOutput         = secretsmanager.CreateSecretOutput
	PutSecretValueInput        = secretsmanager.PutSecretValueInput
	PutSecretValueOutput       = secretsmanager.PutSecretValueOutput
	DeleteSecretInput          = secretsmanager.DeleteSecretInput
	DeleteSecretOutput         = secretsmanager.DeleteSecretOutput
	RestoreSecretInput         = secretsmanager.RestoreSecretInput
	RestoreSecretOutput        = secretsmanager.RestoreSecretOutput
	UpdateSecretInput          = secretsmanager.UpdateSecretInput
	UpdateSecretOutput         = secretsmanager.UpdateSecretOutput
	TagResourceInput           = secretsmanager.TagResourceInput
	TagResourceOutput          = secretsmanager.TagResourceOutput
	UntagResourceInput         = secretsmanager.UntagResourceInput
	UntagResourceOutput        = secretsmanager.UntagResourceOutput
	DescribeSecretInput        = secretsmanager.DescribeSecretInput
	DescribeSecretOutput       = secretsmanager.DescribeSecretOutput
)

// Re-exported Secrets Manager model types.
type (
	SecretVersionsListEntry = types.SecretVersionsListEntry
	SecretListEntry         = types.SecretListEntry
	Tag                     = types.Tag
	Filter                  = types.Filter
	FilterNameStringType    = types.FilterNameStringType
)

// Re-exported Secrets Manager constants.
const (
	FilterNameStringTypeName = types.FilterNameStringTypeName
)

// Re-exported Secrets Manager error types.
//
//nolint:errname // This is a type alias to AWS SDK type, preserving original name for consistency
type (
	ResourceNotFoundException = types.ResourceNotFoundException
)

// Re-exported Secrets Manager functions.
var (
	NewFromConfig           = secretsmanager.NewFromConfig
	NewListSecretsPaginator = secretsmanager.NewListSecretsPaginator
)
