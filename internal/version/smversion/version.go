// Package smversion provides version resolution for AWS Secrets Manager.
package smversion

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/mpyw/suve/internal/api/smapi"
)

// Client is the interface for GetSecretWithVersion.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client Client, spec *Spec) (*secretsmanager.GetSecretValueOutput, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(spec.Name),
	}

	if spec.HasShift() {
		return getSecretWithShift(ctx, client, spec)
	}

	// No shift: use ID or Label directly
	if spec.ID != nil {
		input.VersionId = spec.ID
	}
	if spec.Label != nil {
		input.VersionStage = spec.Label
	}

	return client.GetSecretValue(ctx, input)
}

func getSecretWithShift(ctx context.Context, client Client, spec *Spec) (*secretsmanager.GetSecretValueOutput, error) {
	versions, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId: aws.String(spec.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	versionList := versions.Versions
	if len(versionList) == 0 {
		return nil, fmt.Errorf("secret not found or has no versions: %s", spec.Name)
	}

	sort.Slice(versionList, func(i, j int) bool {
		if versionList[i].CreatedDate == nil {
			return false
		}
		if versionList[j].CreatedDate == nil {
			return true
		}
		return versionList[i].CreatedDate.After(*versionList[j].CreatedDate)
	})

	// Find base index
	baseIdx := 0
	if spec.ID != nil {
		found := false
		for i, v := range versionList {
			if aws.ToString(v.VersionId) == *spec.ID {
				baseIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("version ID not found: %s", *spec.ID)
		}
	} else if spec.Label != nil {
		found := false
		for i, v := range versionList {
			for _, stage := range v.VersionStages {
				if stage == *spec.Label {
					baseIdx = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("version label not found: %s", *spec.Label)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(versionList) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	targetVersion := versionList[targetIdx]
	return client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:  aws.String(spec.Name),
		VersionId: targetVersion.VersionId,
	})
}
