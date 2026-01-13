// Package paramversion provides version resolution for AWS Systems Manager Parameter Store.
package paramversion

import (
	"context"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// Client is the interface for GetParameterWithVersion.
type Client interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// GetParameterWithVersion retrieves a parameter with version/shift support.
// SecureString values are always decrypted.
func GetParameterWithVersion(ctx context.Context, client Client, spec *Spec) (*paramapi.ParameterHistory, error) {
	if spec.HasShift() {
		return getParameterWithShift(ctx, client, spec)
	}

	return getParameterDirect(ctx, client, spec)
}

func getParameterWithShift(ctx context.Context, client paramapi.GetParameterHistoryAPI, spec *Spec) (*paramapi.ParameterHistory, error) {
	history, err := client.GetParameterHistory(ctx, &paramapi.GetParameterHistoryInput{
		Name:           lo.ToPtr(spec.Name),
		WithDecryption: lo.ToPtr(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(history.Parameters) == 0 {
		return nil, fmt.Errorf("parameter not found: %s", spec.Name)
	}

	// Reverse to get newest first
	params := history.Parameters
	slices.Reverse(params)

	baseIdx := 0

	if spec.Absolute.Version != nil {
		var found bool

		_, baseIdx, found = lo.FindIndexOf(params, func(p paramapi.ParameterHistory) bool {
			return p.Version == *spec.Absolute.Version
		})
		if !found {
			return nil, fmt.Errorf("version %d not found", *spec.Absolute.Version)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(params) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	return &params[targetIdx], nil
}

func getParameterDirect(ctx context.Context, client paramapi.GetParameterAPI, spec *Spec) (*paramapi.ParameterHistory, error) {
	var nameWithVersion string
	if spec.Absolute.Version != nil {
		nameWithVersion = fmt.Sprintf("%s:%d", spec.Name, *spec.Absolute.Version)
	} else {
		nameWithVersion = spec.Name
	}

	result, err := client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name:           lo.ToPtr(nameWithVersion),
		WithDecryption: lo.ToPtr(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	param := result.Parameter

	return &paramapi.ParameterHistory{
		Name:             param.Name,
		Value:            param.Value,
		Type:             param.Type,
		Version:          param.Version,
		LastModifiedDate: param.LastModifiedDate,
	}, nil
}
