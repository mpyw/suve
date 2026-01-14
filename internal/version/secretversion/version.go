// Package secretversion provides version resolution for AWS Secrets Manager.
package secretversion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// Client is the interface for GetSecretWithVersion.
type Client interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIDsAPI
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client Client, spec *Spec) (*secretapi.GetSecretValueOutput, error) {
	input := &secretapi.GetSecretValueInput{
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
// versionIDDisplayLength is the number of characters to display for version IDs.
const versionIDDisplayLength = 8

// TruncateVersionID truncates a version ID to a readable short form.
func TruncateVersionID(id string) string {
	if len(id) > versionIDDisplayLength {
		return id[:versionIDDisplayLength]
	}

	return id
}

func getSecretWithShift(ctx context.Context, client Client, spec *Spec) (*secretapi.GetSecretValueOutput, error) {
	versions, err := client.ListSecretVersionIds(ctx, &secretapi.ListSecretVersionIDsInput{
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

	var (
		predicate func(secretapi.SecretVersionsListEntry) bool
		errMsg    string
		found     bool
	)

	switch {
	case spec.Absolute.ID != nil:
		predicate = func(v secretapi.SecretVersionsListEntry) bool {
			return lo.FromPtr(v.VersionId) == *spec.Absolute.ID
		}
		errMsg = fmt.Sprintf("version ID not found: %s", *spec.Absolute.ID)
	case spec.Absolute.Label != nil:
		predicate = func(v secretapi.SecretVersionsListEntry) bool {
			return slices.Contains(v.VersionStages, *spec.Absolute.Label)
		}
		errMsg = fmt.Sprintf("version label not found: %s", *spec.Absolute.Label)
	}

	if predicate != nil {
		_, baseIdx, found = lo.FindIndexOf(versionList, predicate)
		if !found {
			return nil, errors.New(errMsg)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(versionList) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	targetVersion := versionList[targetIdx]

	return client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
		SecretId:  lo.ToPtr(spec.Name),
		VersionId: targetVersion.VersionId,
	})
}
