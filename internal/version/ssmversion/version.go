// Package ssmversion provides version resolution for AWS Systems Manager Parameter Store.
package ssmversion

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/ssmapi"
)

// Client is the interface for GetParameterWithVersion.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// GetParameterWithVersion retrieves a parameter with version/shift support.
func GetParameterWithVersion(ctx context.Context, client Client, spec *Spec, decrypt bool) (*types.ParameterHistory, error) {
	if spec.HasShift() {
		return getParameterWithShift(ctx, client, spec, decrypt)
	}
	return getParameterDirect(ctx, client, spec, decrypt)
}

func getParameterWithShift(ctx context.Context, client ssmapi.GetParameterHistoryAPI, spec *Spec, decrypt bool) (*types.ParameterHistory, error) {
	history, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           lo.ToPtr(spec.Name),
		WithDecryption: lo.ToPtr(decrypt),
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
		_, idx, found := lo.FindIndexOf(params, func(p types.ParameterHistory) bool {
			return p.Version == *spec.Absolute.Version
		})
		if !found {
			return nil, fmt.Errorf("version %d not found", *spec.Absolute.Version)
		}
		baseIdx = idx
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(params) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	return &params[targetIdx], nil
}

func getParameterDirect(ctx context.Context, client ssmapi.GetParameterAPI, spec *Spec, decrypt bool) (*types.ParameterHistory, error) {
	var nameWithVersion string
	if spec.Absolute.Version != nil {
		nameWithVersion = fmt.Sprintf("%s:%d", spec.Name, *spec.Absolute.Version)
	} else {
		nameWithVersion = spec.Name
	}

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           lo.ToPtr(nameWithVersion),
		WithDecryption: lo.ToPtr(decrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	param := result.Parameter
	return &types.ParameterHistory{
		Name:             param.Name,
		Value:            param.Value,
		Type:             param.Type,
		Version:          param.Version,
		LastModifiedDate: param.LastModifiedDate,
	}, nil
}
