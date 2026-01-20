// Package provider defines provider-agnostic interfaces for cloud services.
package provider

import (
	"context"

	"github.com/mpyw/suve/internal/model"
)

// ============================================================================
// UseCase Layer Interfaces
// ============================================================================

// ParameterReader provides read access to parameters.
type ParameterReader interface {
	// GetParameter retrieves a parameter by name and optional version.
	// If version is empty, returns the latest version.
	GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error)

	// GetParameterHistory retrieves all versions of a parameter.
	GetParameterHistory(ctx context.Context, name string) (*model.ParameterHistory, error)

	// ListParameters lists parameters matching the given path prefix.
	// If recursive is true, includes parameters in nested paths.
	ListParameters(ctx context.Context, path string, recursive bool) ([]*model.ParameterListItem, error)
}

// ParameterWriter provides write access to parameters.
type ParameterWriter interface {
	// PutParameter creates or updates a parameter.
	// If overwrite is false and the parameter exists, returns an error.
	PutParameter(ctx context.Context, param *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error)

	// DeleteParameter deletes a parameter by name.
	DeleteParameter(ctx context.Context, name string) error
}

// ParameterTagger provides tag management for parameters.
//
//nolint:iface // Intentionally similar to SecretTagger but separate for clarity.
type ParameterTagger interface {
	// GetTags retrieves all tags for a parameter.
	GetTags(ctx context.Context, name string) (map[string]string, error)

	// AddTags adds or updates tags on a parameter.
	AddTags(ctx context.Context, name string, tags map[string]string) error

	// RemoveTags removes tags from a parameter by key names.
	RemoveTags(ctx context.Context, name string, keys []string) error
}

// ParameterService combines all parameter operations.
type ParameterService interface {
	ParameterReader
	ParameterWriter
	ParameterTagger
}

// ============================================================================
// Provider Layer Interfaces (Generic)
// ============================================================================

// TypedParameterReader provides type-safe access to parameters with metadata.
// This is used internally by provider adapters.
type TypedParameterReader[M any] interface {
	// GetTypedParameter retrieves a parameter with provider-specific metadata.
	GetTypedParameter(ctx context.Context, name string, version string) (*model.TypedParameter[M], error)

	// GetTypedParameterHistory retrieves all versions with provider-specific metadata.
	GetTypedParameterHistory(ctx context.Context, name string) (*model.TypedParameterHistory[M], error)
}

// ============================================================================
// Adapter Helpers
// ============================================================================

// WrapTypedParameterReader wraps a TypedParameterReader to implement ParameterReader.
func WrapTypedParameterReader[M any](r TypedParameterReader[M]) ParameterReader {
	return &typedParameterReaderAdapter[M]{inner: r}
}

type typedParameterReaderAdapter[M any] struct {
	inner TypedParameterReader[M]
}

func (a *typedParameterReaderAdapter[M]) GetParameter(
	ctx context.Context, name string, version string,
) (*model.Parameter, error) {
	p, err := a.inner.GetTypedParameter(ctx, name, version)
	if err != nil {
		return nil, err
	}

	return p.ToBase(), nil
}

func (a *typedParameterReaderAdapter[M]) GetParameterHistory(
	ctx context.Context, name string,
) (*model.ParameterHistory, error) {
	h, err := a.inner.GetTypedParameterHistory(ctx, name)
	if err != nil {
		return nil, err
	}

	return h.ToBase(), nil
}

func (a *typedParameterReaderAdapter[M]) ListParameters(
	_ context.Context, _ string, _ bool,
) ([]*model.ParameterListItem, error) {
	// TypedParameterReader doesn't include list functionality,
	// so this adapter cannot implement ListParameters.
	// Concrete implementations should implement ParameterReader directly.
	return nil, nil
}
