// Package smversion provides version resolution for AWS Secrets Manager.
package smversion

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"

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
		SecretId: lo.ToPtr(spec.Name),
	}

	if spec.HasShift() {
		return getSecretWithShift(ctx, client, spec)
	}

	// No shift: use ID or Label directly
	if spec.Absolute.ID != nil {
		input.VersionId = spec.Absolute.ID
	}
	if spec.Absolute.Label != nil {
		input.VersionStage = spec.Absolute.Label
	}

	return client.GetSecretValue(ctx, input)
}

// TruncateVersionID truncates a version ID to 8 characters for display.
// Secrets Manager version IDs are UUIDs which are long; this provides
// a readable short form similar to git commit hashes.
func TruncateVersionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func getSecretWithShift(ctx context.Context, client Client, spec *Spec) (*secretsmanager.GetSecretValueOutput, error) {
	versions, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId: lo.ToPtr(spec.Name),
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
	switch {
	case spec.Absolute.ID != nil:
		_, idx, found := lo.FindIndexOf(versionList, func(v types.SecretVersionsListEntry) bool {
			return lo.FromPtr(v.VersionId) == *spec.Absolute.ID
		})
		if !found {
			return nil, fmt.Errorf("version ID not found: %s", *spec.Absolute.ID)
		}
		baseIdx = idx
	case spec.Absolute.Label != nil:
		_, idx, found := lo.FindIndexOf(versionList, func(v types.SecretVersionsListEntry) bool {
			return lo.Contains(v.VersionStages, *spec.Absolute.Label)
		})
		if !found {
			return nil, fmt.Errorf("version label not found: %s", *spec.Absolute.Label)
		}
		baseIdx = idx
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(versionList) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	targetVersion := versionList[targetIdx]
	return client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:  lo.ToPtr(spec.Name),
		VersionId: targetVersion.VersionId,
	})
}
