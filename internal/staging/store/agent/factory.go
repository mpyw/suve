package agent

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// Factory creates agent stores for staging state.
// It implements store.AgentStoreFactory.
type Factory struct {
	core store.AgentStore //nolint:staticcheck // using legacy interface to wrap
}

// NewFactory creates a new agent store factory.
func NewFactory(accountID, region string, opts ...StoreOption) *Factory {
	return &Factory{
		core: NewStore(accountID, region, opts...),
	}
}

// Service returns a store for a specific service.
func (f *Factory) Service(service staging.Service) store.ServiceStore {
	return &serviceStore{
		core:    f.core,
		service: service,
	}
}

// Global returns a store for all services.
func (f *Factory) Global() store.GlobalStore {
	return &globalStore{core: f.core}
}

// Ping checks if the agent daemon is running.
func (f *Factory) Ping(ctx context.Context) error {
	return f.core.Ping(ctx)
}

// Start ensures the agent daemon is running, starting it if necessary.
func (f *Factory) Start(ctx context.Context) error {
	return f.core.Start(ctx)
}

// serviceStore wraps AgentStore to provide service-scoped access.
type serviceStore struct {
	core    store.AgentStore //nolint:staticcheck // using legacy interface to wrap
	service staging.Service
}

// GetEntry retrieves a staged entry by name.
func (s *serviceStore) GetEntry(ctx context.Context, name string) (*staging.Entry, error) {
	return s.core.GetEntry(ctx, s.service, name)
}

// GetTag retrieves staged tag changes by name.
func (s *serviceStore) GetTag(ctx context.Context, name string) (*staging.TagEntry, error) {
	return s.core.GetTag(ctx, s.service, name)
}

// ListEntries returns all staged entries for this service.
func (s *serviceStore) ListEntries(ctx context.Context) (map[string]staging.Entry, error) {
	all, err := s.core.ListEntries(ctx, s.service)
	if err != nil {
		return nil, err
	}

	entries := all[s.service]
	if entries == nil {
		entries = make(map[string]staging.Entry)
	}

	return entries, nil
}

// ListTags returns all staged tag changes for this service.
func (s *serviceStore) ListTags(ctx context.Context) (map[string]staging.TagEntry, error) {
	all, err := s.core.ListTags(ctx, s.service)
	if err != nil {
		return nil, err
	}

	tags := all[s.service]
	if tags == nil {
		tags = make(map[string]staging.TagEntry)
	}

	return tags, nil
}

// StageEntry adds or updates a staged entry.
func (s *serviceStore) StageEntry(ctx context.Context, name string, entry staging.Entry) error {
	return s.core.StageEntry(ctx, s.service, name, entry)
}

// StageTag adds or updates staged tag changes.
func (s *serviceStore) StageTag(ctx context.Context, name string, tagEntry staging.TagEntry) error {
	return s.core.StageTag(ctx, s.service, name, tagEntry)
}

// UnstageEntry removes a staged entry.
func (s *serviceStore) UnstageEntry(ctx context.Context, name string) error {
	return s.core.UnstageEntry(ctx, s.service, name)
}

// UnstageTag removes staged tag changes.
func (s *serviceStore) UnstageTag(ctx context.Context, name string) error {
	return s.core.UnstageTag(ctx, s.service, name)
}

// UnstageAll removes all staged changes for this service.
func (s *serviceStore) UnstageAll(ctx context.Context) error {
	return s.core.UnstageAll(ctx, s.service)
}

// Drain retrieves state for this service.
func (s *serviceStore) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return s.core.Drain(ctx, s.service, keep)
}

// WriteState writes state for this service.
func (s *serviceStore) WriteState(ctx context.Context, state *staging.State) error {
	return s.core.WriteState(ctx, s.service, state)
}

// UnstageEntryWithHint removes a staged entry with an operation hint.
func (s *serviceStore) UnstageEntryWithHint(ctx context.Context, name string, hint string) error {
	if unstager, ok := s.core.(store.HintedUnstager); ok { //nolint:staticcheck // using legacy interface
		return unstager.UnstageEntryWithHint(ctx, s.service, name, hint)
	}

	return s.core.UnstageEntry(ctx, s.service, name)
}

// UnstageTagWithHint removes staged tag changes with an operation hint.
func (s *serviceStore) UnstageTagWithHint(ctx context.Context, name string, hint string) error {
	if unstager, ok := s.core.(store.HintedUnstager); ok { //nolint:staticcheck // using legacy interface
		return unstager.UnstageTagWithHint(ctx, s.service, name, hint)
	}

	return s.core.UnstageTag(ctx, s.service, name)
}

// UnstageAllWithHint removes all staged changes with an operation hint.
func (s *serviceStore) UnstageAllWithHint(ctx context.Context, hint string) error {
	if unstager, ok := s.core.(store.HintedUnstager); ok { //nolint:staticcheck // using legacy interface
		return unstager.UnstageAllWithHint(ctx, s.service, hint)
	}

	return s.core.UnstageAll(ctx, s.service)
}

// globalStore wraps AgentStore to provide global access.
type globalStore struct {
	core store.AgentStore //nolint:staticcheck // using legacy interface to wrap
}

// ListEntries returns all staged entries across all services.
func (g *globalStore) ListEntries(ctx context.Context) (map[staging.Service]map[string]staging.Entry, error) {
	return g.core.ListEntries(ctx, "")
}

// ListTags returns all staged tag changes across all services.
func (g *globalStore) ListTags(ctx context.Context) (map[staging.Service]map[string]staging.TagEntry, error) {
	return g.core.ListTags(ctx, "")
}

// UnstageAll removes all staged changes across all services.
func (g *globalStore) UnstageAll(ctx context.Context) error {
	return g.core.UnstageAll(ctx, "")
}

// Drain retrieves state for all services.
func (g *globalStore) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return g.core.Drain(ctx, "", keep)
}

// WriteState writes state for all services.
func (g *globalStore) WriteState(ctx context.Context, state *staging.State) error {
	return g.core.WriteState(ctx, "", state)
}

// UnstageAllWithHint removes all staged changes across all services with an operation hint.
func (g *globalStore) UnstageAllWithHint(ctx context.Context, hint string) error {
	if unstager, ok := g.core.(store.HintedUnstager); ok { //nolint:staticcheck // using legacy interface
		return unstager.UnstageAllWithHint(ctx, "", hint)
	}

	return g.core.UnstageAll(ctx, "")
}

// Compile-time checks.
var (
	_ store.AgentStoreFactory     = (*Factory)(nil)
	_ store.ServiceStore          = (*serviceStore)(nil)
	_ store.GlobalStore           = (*globalStore)(nil)
	_ store.HintedServiceUnstager = (*serviceStore)(nil)
	_ store.HintedGlobalUnstager  = (*globalStore)(nil)
)
