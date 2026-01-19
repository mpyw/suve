package paramversion

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/internal"
)

// Resolve resolves a parameter version specification to a concrete parameter.
// It fetches the parameter value, applying shift if specified.
func Resolve(
	ctx context.Context,
	client provider.ParameterReader,
	spec *Spec,
) (*model.Parameter, error) {
	if spec.HasShift() {
		return resolveWithShift(ctx, client, spec)
	}

	return resolveDirect(ctx, client, spec)
}

func resolveWithShift(
	ctx context.Context,
	client provider.ParameterReader,
	spec *Spec,
) (*model.Parameter, error) {
	history, err := client.GetParameterHistory(ctx, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(history.Parameters) == 0 {
		return nil, fmt.Errorf("parameter not found: %s", spec.Name)
	}

	// Sort by version descending (newest first)
	params := history.Parameters
	slices.SortFunc(params, func(a, b *model.Parameter) int {
		va, _ := parseVersionString(a.Version)
		vb, _ := parseVersionString(b.Version)

		return int(vb - va) // Descending order
	})

	baseIdx := 0

	if spec.Absolute.Version != nil {
		var found bool

		targetVersion := *spec.Absolute.Version

		for i, p := range params {
			v, _ := parseVersionString(p.Version)
			if v == targetVersion {
				baseIdx = i
				found = true

				break
			}
		}

		if !found {
			return nil, fmt.Errorf("version %d not found", targetVersion)
		}
	}

	targetIdx, err := internal.ApplyShift(baseIdx, spec.Shift, len(params))
	if err != nil {
		return nil, err
	}

	return params[targetIdx], nil
}

func resolveDirect(
	ctx context.Context,
	client provider.ParameterReader,
	spec *Spec,
) (*model.Parameter, error) {
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

// parseVersionString converts a version string to int64.
func parseVersionString(version string) (int64, error) {
	if version == "" {
		return 0, nil
	}

	return strconv.ParseInt(version, 10, 64)
}
