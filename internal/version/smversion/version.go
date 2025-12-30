// Package smversion provides version resolution for AWS Secrets Manager.
package smversion

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/version"
)

// Client is the interface for GetSecretWithVersion.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client Client, spec *version.Spec) (*secretsmanager.GetSecretValueOutput, error) {
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
