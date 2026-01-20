// Package paramversion provides version resolution for AWS Systems Manager Parameter Store.
package paramversion

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// GetParameterWithVersion retrieves a parameter with version/shift support.
// SecureString values are always decrypted.
func GetParameterWithVersion(ctx context.Context, client provider.ParameterReader, spec *Spec) (*model.Parameter, error) {
	if spec.HasShift() {
		return getParameterWithShift(ctx, client, spec)
	}

	return getParameterDirect(ctx, client, spec)
}

func getParameterWithShift(ctx context.Context, client provider.ParameterReader, spec *Spec) (*model.Parameter, error) {
	history, err := client.GetParameterHistory(ctx, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(history.Parameters) == 0 {
		return nil, fmt.Errorf("parameter not found: %s", spec.Name)
	}

	// Copy and reverse to get newest first
	params := make([]*model.Parameter, len(history.Parameters))
	copy(params, history.Parameters)
	slices.Reverse(params)

	baseIdx := 0

	if spec.Absolute.Version != nil {
		targetVersion := strconv.FormatInt(*spec.Absolute.Version, 10)
		found := false

		for i, p := range params {
			if p.Version == targetVersion {
				baseIdx = i
				found = true

				break
			}
		}

		if !found {
			return nil, fmt.Errorf("version %d not found", *spec.Absolute.Version)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(params) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	return params[targetIdx], nil
}

func getParameterDirect(ctx context.Context, client provider.ParameterReader, spec *Spec) (*model.Parameter, error) {
	version := ""
	if spec.Absolute.Version != nil {
		version = strconv.FormatInt(*spec.Absolute.Version, 10)
	}

	param, err := client.GetParameter(ctx, spec.Name, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	return param, nil
}
