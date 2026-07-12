package data

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagedKey identifies a staged item by its (name, namespace) composite, so a
// name staged under several App Configuration namespaces is tracked per
// namespace (empty namespace for the null namespace and every other provider).
type StagedKey struct {
	Name      string
	Namespace string
}

// StagingSnapshot is the browser's read-only view of a service's staged state.
// Keys drives the [staged] badge and the detail banner; DeleteKeys is the subset
// staged for deletion, so the browser can gate the edit/delete/tag affordances
// that the reducer would reject as dead-end transitions (#692). EntryKeys and
// TagKeys split Keys by change kind — a value/entry change and a tag change —
// so the detail banner can distinguish value-only / tag-only / both, mirroring
// the GUI's StagingStatus {hasEntry, hasTags} pair
// (internal/gui/frontend/src/lib/StagingBanner.svelte) (#701). EntryCount and
// TagCount are the staged entry-row and tag-change totals whose sum feeds the
// Staging tab badge — the same entries+tags definition the staging page uses, so
// the badge no longer oscillates between two counts (#693).
type StagingSnapshot struct {
	Keys       map[StagedKey]struct{}
	DeleteKeys map[StagedKey]struct{}
	EntryKeys  map[StagedKey]struct{}
	TagKeys    map[StagedKey]struct{}
	EntryCount int
	TagCount   int
}

// StagingProbe reports which items in the current service have staged changes
// (an entry or a tag change), so the browser can show a [staged] badge and the
// detail pane a staged-changes banner. It is read-only — the parity of the
// GUI's StagingCheckStatus/StagingStatus reads (the staging page owns the mutations).
type StagingProbe interface {
	// Staged returns the staged snapshot (badge keys, delete-staged subset, and
	// entry/tag counts) for the service.
	Staged(ctx context.Context) (StagingSnapshot, error)
}

// StoreUnavailableError marks a StagingProbe failure that comes from CONSTRUCTING
// the on-disk staging store (a keychain hard-fail / key-loss while encrypted state
// exists), as opposed to a transient status read. This class of failure is
// persistent and actionable, so the browser surfaces it on the error line, while
// keeping ordinary probe read errors quiet (badges just do not show). The epic
// requires a key-loss to be visible on the read path, not only the write path.
type StoreUnavailableError struct{ Err error }

func (e *StoreUnavailableError) Error() string { return e.Err.Error() }

func (e *StoreUnavailableError) Unwrap() error { return e.Err }

// stagingProbe wraps the staging StatusUseCase, mirroring the GUI's
// StagingStatus read (strategy parser + read store).
type stagingProbe struct {
	strategy staging.ServiceStrategy
	store    store.ReadOperator
}

// NewStagingProbe builds a StagingProbe over a staging strategy (parser) and a
// read-only staging store, both resolved for the same scope (the invariant the
// GUI's getStagingStoreScoped/getParserScoped pairing keeps).
func NewStagingProbe(strategy staging.ServiceStrategy, store store.ReadOperator) StagingProbe {
	return &stagingProbe{strategy: strategy, store: store}
}

func (p *stagingProbe) Staged(ctx context.Context) (StagingSnapshot, error) {
	uc := &stagingusecase.StatusUseCase{Strategy: p.strategy, Store: p.store}

	out, err := uc.Execute(ctx, stagingusecase.StatusInput{})
	if err != nil {
		return StagingSnapshot{}, err
	}

	snap := StagingSnapshot{
		Keys:       make(map[StagedKey]struct{}, len(out.Entries)+len(out.TagEntries)),
		DeleteKeys: map[StagedKey]struct{}{},
		EntryKeys:  make(map[StagedKey]struct{}, len(out.Entries)),
		TagKeys:    make(map[StagedKey]struct{}, len(out.TagEntries)),
		EntryCount: len(out.Entries),
		TagCount:   len(out.TagEntries),
	}

	for _, e := range out.Entries {
		key := StagedKey{Name: e.Name, Namespace: e.Namespace}
		snap.Keys[key] = struct{}{}
		snap.EntryKeys[key] = struct{}{}

		if e.Operation == staging.OperationDelete {
			snap.DeleteKeys[key] = struct{}{}
		}
	}

	for _, t := range out.TagEntries {
		key := StagedKey{Name: t.Name, Namespace: t.Namespace}
		snap.Keys[key] = struct{}{}
		snap.TagKeys[key] = struct{}{}
	}

	return snap, nil
}
