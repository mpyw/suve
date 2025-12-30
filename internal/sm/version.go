package sm

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/mpyw/suve/internal/version"
)

// GetSecretValueAPI is the interface for getting a secret value.
type GetSecretValueAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// ListSecretVersionIdsAPI is the interface for listing secret versions.
type ListSecretVersionIdsAPI interface {
	ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

// versionedClient is the interface for GetSecretWithVersion.
type versionedClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client versionedClient, spec *version.Spec) (*secretsmanager.GetSecretValueOutput, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(spec.Name),
	}

	if spec.Label != nil {
		input.VersionStage = spec.Label
	}

	if spec.HasShift() {
		versions, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
			SecretId: aws.String(spec.Name),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list versions: %w", err)
		}

		versionList := versions.Versions
		sort.Slice(versionList, func(i, j int) bool {
			if versionList[i].CreatedDate == nil {
				return false
			}
			if versionList[j].CreatedDate == nil {
				return true
			}
			return versionList[i].CreatedDate.After(*versionList[j].CreatedDate)
		})

		if spec.Shift >= len(versionList) {
			return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
		}

		targetVersion := versionList[spec.Shift]
		input.VersionId = targetVersion.VersionId
	}

	return client.GetSecretValue(ctx, input)
}
