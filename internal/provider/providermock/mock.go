// Package providermock provides a configurable mock implementation of the
// provider interfaces (Reader/Writer/Tagger/Store) for use in unit tests.
//
// Each method delegates to an optional function field; when a field is nil the
// method returns ErrNotConfigured so that tests fail loudly if they exercise an
// unset path.
package providermock

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// ErrNotConfigured is returned by a mock method whose function field is nil.
var ErrNotConfigured = errors.New("providermock: method not configured")

// Store is a configurable mock of provider.Store (Reader + Writer + Tagger).
type Store struct {
	ResolveFunc func(ctx context.Context, name, spec string) (provider.VersionRef, error)
	GetFunc     func(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error)
	HistoryFunc func(ctx context.Context, name string) ([]domain.Version, error)
	ListFunc    func(ctx context.Context) ([]string, error)
	CreateFunc  func(
		ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
	) (domain.Version, error)
	PutFunc func(
		ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
	) (domain.Version, error)
	DeleteFunc  func(ctx context.Context, name string, opts ...provider.DeleteOption) error
	TagFunc     func(ctx context.Context, name string, add map[string]string) error
	UntagFunc   func(ctx context.Context, name string, keys []string) error
	RestoreFunc func(ctx context.Context, name string) error
}

// Compile-time assertions that *Store implements the provider contracts.
var (
	_ provider.Store    = (*Store)(nil)
	_ provider.Restorer = (*Store)(nil)
)

// Resolve delegates to ResolveFunc.
func (s *Store) Resolve(ctx context.Context, name, spec string) (provider.VersionRef, error) {
	if s.ResolveFunc == nil {
		return provider.VersionRef{}, ErrNotConfigured
	}

	return s.ResolveFunc(ctx, name, spec)
}

// Get delegates to GetFunc.
func (s *Store) Get(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
	if s.GetFunc == nil {
		return nil, ErrNotConfigured
	}

	return s.GetFunc(ctx, name, ref)
}

// History delegates to HistoryFunc.
func (s *Store) History(ctx context.Context, name string) ([]domain.Version, error) {
	if s.HistoryFunc == nil {
		return nil, ErrNotConfigured
	}

	return s.HistoryFunc(ctx, name)
}

// List delegates to ListFunc.
func (s *Store) List(ctx context.Context) ([]string, error) {
	if s.ListFunc == nil {
		return nil, ErrNotConfigured
	}

	return s.ListFunc(ctx)
}

// Create delegates to CreateFunc.
func (s *Store) Create(
	ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	if s.CreateFunc == nil {
		return domain.Version{}, ErrNotConfigured
	}

	return s.CreateFunc(ctx, name, value, valueType, description, opts...)
}

// Put delegates to PutFunc.
func (s *Store) Put(
	ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	if s.PutFunc == nil {
		return domain.Version{}, ErrNotConfigured
	}

	return s.PutFunc(ctx, name, value, valueType, description, opts...)
}

// Delete delegates to DeleteFunc.
func (s *Store) Delete(ctx context.Context, name string, opts ...provider.DeleteOption) error {
	if s.DeleteFunc == nil {
		return ErrNotConfigured
	}

	return s.DeleteFunc(ctx, name, opts...)
}

// Tag delegates to TagFunc.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if s.TagFunc == nil {
		return ErrNotConfigured
	}

	return s.TagFunc(ctx, name, add)
}

// Untag delegates to UntagFunc.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if s.UntagFunc == nil {
		return ErrNotConfigured
	}

	return s.UntagFunc(ctx, name, keys)
}

// Restore delegates to RestoreFunc.
func (s *Store) Restore(ctx context.Context, name string) error {
	if s.RestoreFunc == nil {
		return ErrNotConfigured
	}

	return s.RestoreFunc(ctx, name)
}
