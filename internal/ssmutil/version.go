// Package ssmutil provides utilities for AWS Systems Manager Parameter Store.
package ssmutil

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/mpyw/suve/internal/ssmapi"
	"github.com/mpyw/suve/internal/version"
)

// versionedClient is the interface for GetParameterWithVersion.
type versionedClient interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// GetParameterWithVersion retrieves a parameter with version/shift support.
func GetParameterWithVersion(ctx context.Context, client versionedClient, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	if spec.HasShift() {
		return getParameterWithShift(ctx, client, spec, decrypt)
	}
	return getParameterDirect(ctx, client, spec, decrypt)
}

func getParameterWithShift(ctx context.Context, client ssmapi.GetParameterHistoryAPI, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	history, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(spec.Name),
		WithDecryption: aws.Bool(decrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(history.Parameters) == 0 {
		return nil, fmt.Errorf("parameter not found: %s", spec.Name)
	}

	// Reverse to get newest first
	params := history.Parameters
	for i, j := 0, len(params)-1; i < j; i, j = i+1, j-1 {
		params[i], params[j] = params[j], params[i]
	}

	baseIdx := 0
	if spec.Version != nil {
		found := false
		for i, p := range params {
			if p.Version == *spec.Version {
				baseIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("version %d not found", *spec.Version)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(params) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	return &params[targetIdx], nil
}

func getParameterDirect(ctx context.Context, client ssmapi.GetParameterAPI, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	var nameWithVersion string
	if spec.Version != nil {
		nameWithVersion = fmt.Sprintf("%s:%d", spec.Name, *spec.Version)
	} else {
		nameWithVersion = spec.Name
	}

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(nameWithVersion),
		WithDecryption: aws.Bool(decrypt),
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
