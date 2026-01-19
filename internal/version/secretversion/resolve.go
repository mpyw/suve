package secretversion

import (
	"context"
	"fmt"
	"slices"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/internal"
)

// Resolve resolves a secret version specification to a concrete secret.
// It fetches the secret value, applying shift if specified.
func Resolve(
	ctx context.Context,
	client provider.SecretReader,
	spec *Spec,
) (*model.Secret, error) {
	if spec.HasShift() {
		return resolveWithShift(ctx, client, spec)
	}

	return resolveDirect(ctx, client, spec)
}

func resolveWithShift(
	ctx context.Context,
	client provider.SecretReader,
	spec *Spec,
) (*model.Secret, error) {
	versions, err := client.GetSecretVersions(ctx, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret versions: %w", err)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("secret not found: %s", spec.Name)
	}

	// Sort by created date descending (newest first)
	slices.SortFunc(versions, func(a, b *model.SecretVersion) int {
		if a.CreatedDate == nil && b.CreatedDate == nil {
			return 0
		}

		if a.CreatedDate == nil {
			return 1
		}

		if b.CreatedDate == nil {
			return -1
		}

		return b.CreatedDate.Compare(*a.CreatedDate)
	})

	baseIdx, err := findBaseIndex(versions, spec)
	if err != nil {
		return nil, err
	}

	targetIdx, err := internal.ApplyShift(baseIdx, spec.Shift, len(versions))
	if err != nil {
		return nil, err
	}

	// Get the full secret value for the resolved version
	return client.GetSecret(ctx, spec.Name, versions[targetIdx].VersionID, "")
}

func findBaseIndex(versions []*model.SecretVersion, spec *Spec) (int, error) {
	// Default to latest (index 0)
	if spec.Absolute.ID == nil && spec.Absolute.Label == nil {
		return 0, nil
	}

	for i, v := range versions {
		// Match by version ID
		if spec.Absolute.ID != nil && v.VersionID == *spec.Absolute.ID {
			return i, nil
		}

		// Match by label (AWS-specific: check VersionStages in metadata)
		if spec.Absolute.Label != nil {
			if meta, ok := v.Metadata.(model.AWSSecretMeta); ok {
				if slices.Contains(meta.VersionStages, *spec.Absolute.Label) {
					return i, nil
				}
			}
		}
	}

	// Not found
	if spec.Absolute.ID != nil {
		return 0, fmt.Errorf("version ID %s not found", *spec.Absolute.ID)
	}

	return 0, fmt.Errorf("staging label %s not found", *spec.Absolute.Label)
}

func resolveDirect(
	ctx context.Context,
	client provider.SecretReader,
	spec *Spec,
) (*model.Secret, error) {
	versionID := ""
	versionStage := ""

	if spec.Absolute.ID != nil {
		versionID = *spec.Absolute.ID
	}

	if spec.Absolute.Label != nil {
		versionStage = *spec.Absolute.Label
	}

	secret, err := client.GetSecret(ctx, spec.Name, versionID, versionStage)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	return secret, nil
}
