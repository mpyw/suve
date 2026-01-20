// Package secretversion provides version resolution for AWS Secrets Manager.
package secretversion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// Client is the interface for GetSecretWithVersion.
// It requires SecretReader methods for version resolution.
type Client interface {
	provider.SecretReader
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client Client, spec *Spec) (*model.Secret, error) {
	if spec.HasShift() {
		return getSecretWithShift(ctx, client, spec)
	}

	// No shift: use ID or Label directly
	versionID := ""
	versionStage := ""

	if spec.Absolute.ID != nil {
		versionID = *spec.Absolute.ID
	}

	if spec.Absolute.Label != nil {
		versionStage = *spec.Absolute.Label
	}

	return client.GetSecret(ctx, spec.Name, versionID, versionStage)
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

func getSecretWithShift(ctx context.Context, client Client, spec *Spec) (*model.Secret, error) {
	versions, err := client.GetSecretVersions(ctx, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("secret not found or has no versions: %s", spec.Name)
	}

	// Sort by CreatedDate descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedAt == nil {
			return false
		}

		if versions[j].CreatedAt == nil {
			return true
		}

		return versions[i].CreatedAt.After(*versions[j].CreatedAt)
	})

	// Find base index
	baseIdx := 0

	var (
		predicate func(*model.SecretVersion) bool
		errMsg    string
		found     bool
	)

	switch {
	case spec.Absolute.ID != nil:
		predicate = func(v *model.SecretVersion) bool {
			return v.Version == *spec.Absolute.ID
		}
		errMsg = fmt.Sprintf("version ID not found: %s", *spec.Absolute.ID)
	case spec.Absolute.Label != nil:
		predicate = func(v *model.SecretVersion) bool {
			if meta, ok := v.Metadata.(model.AWSSecretVersionMeta); ok {
				return slices.Contains(meta.VersionStages, *spec.Absolute.Label)
			}

			return false
		}
		errMsg = fmt.Sprintf("version label not found: %s", *spec.Absolute.Label)
	}

	if predicate != nil {
		_, baseIdx, found = lo.FindIndexOf(versions, predicate)
		if !found {
			return nil, errors.New(errMsg)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(versions) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	targetVersion := versions[targetIdx]

	return client.GetSecret(ctx, spec.Name, targetVersion.Version, "")
}
