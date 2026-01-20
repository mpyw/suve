// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// ShowClient is the interface for the show use case.
type ShowClient interface {
	provider.ParameterReader
	provider.ParameterTagger
}

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *paramversion.Spec
}

// ShowTag represents a tag key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	Value        string
	Version      string
	Type         string // Parameter type (e.g., "String", "SecureString")
	Description  string
	LastModified *time.Time
	Tags         []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Client ShowClient
}

// Execute runs the show use case.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	param, err := u.getParameterWithVersion(ctx, input.Spec)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:         param.Name,
		Value:        param.Value,
		Version:      param.Version,
		Description:  param.Description,
		LastModified: param.LastModified,
	}

	// Extract Type from AWS metadata if available
	if meta := param.AWSMeta(); meta != nil {
		output.Type = meta.Type
	}

	// Fetch tags
	tags, err := u.Client.GetTags(ctx, param.Name)
	if err == nil && tags != nil {
		for k, v := range tags {
			output.Tags = append(output.Tags, ShowTag{Key: k, Value: v})
		}
	}

	return output, nil
}

// getParameterWithVersion retrieves a parameter with version/shift support.
func (u *ShowUseCase) getParameterWithVersion(
	ctx context.Context, spec *paramversion.Spec,
) (*model.Parameter, error) {
	if spec.HasShift() {
		return u.getParameterWithShift(ctx, spec)
	}

	return u.getParameterDirect(ctx, spec)
}

func (u *ShowUseCase) getParameterWithShift(
	ctx context.Context, spec *paramversion.Spec,
) (*model.Parameter, error) {
	history, err := u.Client.GetParameterHistory(ctx, spec.Name)
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

func (u *ShowUseCase) getParameterDirect(
	ctx context.Context, spec *paramversion.Spec,
) (*model.Parameter, error) {
	version := ""
	if spec.Absolute.Version != nil {
		version = strconv.FormatInt(*spec.Absolute.Version, 10)
	}

	param, err := u.Client.GetParameter(ctx, spec.Name, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	return param, nil
}
