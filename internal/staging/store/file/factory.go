package file

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// Factory creates file stores for staging state.
// It implements store.FileStoreFactory.
type Factory struct {
	stores map[staging.Service]*Store
}

// NewFactory creates a new file store factory for all services.
func NewFactory(accountID, region string) (*Factory, error) {
	stores, err := NewStoresForAllServices(accountID, region)
	if err != nil {
		return nil, err
	}

	return &Factory{stores: stores}, nil
}

// NewFactoryWithPassphrase creates a new file store factory with a passphrase.
func NewFactoryWithPassphrase(accountID, region, passphrase string) (*Factory, error) {
	stores, err := NewStoresWithPassphrase(accountID, region, passphrase)
	if err != nil {
		return nil, err
	}

	return &Factory{stores: stores}, nil
}

// NewFactoryFromStores creates a factory from pre-created stores.
// This is useful when stores have already been created with passphrase handling.
func NewFactoryFromStores(stores map[staging.Service]*Store) *Factory {
	return &Factory{stores: stores}
}

// Service returns a store for a specific service.
func (f *Factory) Service(service staging.Service) store.DrainWriter {
	s, ok := f.stores[service]
	if !ok {
		return nil
	}

	return &serviceDrainWriter{store: s}
}

// Global returns a store for all services.
func (f *Factory) Global() store.DrainWriter {
	return &globalDrainWriter{stores: f.stores}
}

// Exists checks if any stash file exists.
func (f *Factory) Exists() (bool, error) {
	return AnyExists(f.stores)
}

// Encrypted checks if any stash file is encrypted.
func (f *Factory) Encrypted() (bool, error) {
	return AnyEncrypted(f.stores)
}

// Delete deletes all stash files.
func (f *Factory) Delete() error {
	return DeleteAll(f.stores)
}

// Stores returns the underlying stores for iteration.
// This is used when callers need direct access to individual stores.
func (f *Factory) Stores() map[staging.Service]*Store {
	return f.stores
}

// serviceDrainWriter wraps a single Store to implement store.DrainWriter.
type serviceDrainWriter struct {
	store *Store
}

// Drain reads state from this service's file.
func (s *serviceDrainWriter) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return s.store.Drain(ctx, "", keep)
}

// WriteState writes state to this service's file.
func (s *serviceDrainWriter) WriteState(ctx context.Context, state *staging.State) error {
	return s.store.WriteState(ctx, "", state)
}

// globalDrainWriter wraps multiple Stores to implement store.DrainWriter.
type globalDrainWriter struct {
	stores map[staging.Service]*Store
}

// Drain reads and merges state from all service files.
func (g *globalDrainWriter) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return DrainAll(ctx, g.stores, keep)
}

// WriteState writes state to all service files.
func (g *globalDrainWriter) WriteState(ctx context.Context, state *staging.State) error {
	return WriteAll(ctx, g.stores, state)
}

// Compile-time checks.
var (
	_ store.FileStoreFactory = (*Factory)(nil)
	_ store.DrainWriter      = (*serviceDrainWriter)(nil)
	_ store.DrainWriter      = (*globalDrainWriter)(nil)
)
