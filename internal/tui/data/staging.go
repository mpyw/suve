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

// StagingProbe reports which items in the current service have staged changes
// (an entry or a tag change), so the browser can show a [staged] badge and the
// detail pane a staged-changes banner. It is read-only — the parity of the
// GUI's StagingCheckStatus/StagingStatus reads (#Step-5 owns the mutations).
type StagingProbe interface {
	// StagedKeys returns the set of staged (name, namespace) keys for the service.
	StagedKeys(ctx context.Context) (map[StagedKey]struct{}, error)
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

func (p *stagingProbe) StagedKeys(ctx context.Context) (map[StagedKey]struct{}, error) {
	uc := &stagingusecase.StatusUseCase{Strategy: p.strategy, Store: p.store}

	out, err := uc.Execute(ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	keys := make(map[StagedKey]struct{}, len(out.Entries)+len(out.TagEntries))

	for _, e := range out.Entries {
		keys[StagedKey{Name: e.Name, Namespace: e.Namespace}] = struct{}{}
	}

	for _, t := range out.TagEntries {
		keys[StagedKey{Name: t.Name, Namespace: t.Namespace}] = struct{}{}
	}

	return keys, nil
}
